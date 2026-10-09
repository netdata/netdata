import com.sun.tools.attach.VirtualMachine;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.StandardOpenOption;
import java.nio.file.attribute.PosixFilePermissions;
import java.util.Arrays;
import java.util.HashSet;
import java.util.Set;
import java.util.concurrent.TimeUnit;

/** Lab only: owned PID namespace, or a strictly bounded task cgroup on an authorized host. */
public class Scout {
    static Path home = Path.of("/lab");
    static Path state = Path.of("/state");
    static String cgroupPrefix;
    static String javaCommand = "java";

    static boolean inCgroup(String membership, String prefix) {
        return membership.lines().anyMatch(line -> line.equals("0::" + prefix)
                || line.startsWith("0::" + prefix + "/"));
    }

    static void configure() {
        String scope = System.getenv("SCOUT_LAB_SCOPE");
        if ("owned-fixture-pid-namespace".equals(scope)) return;
        String run = System.getenv("SCOUT_RUN");
        if (!"owned-host-cgroup".equals(scope) || run == null || !run.matches("[a-f0-9]{12}")) {
            throw new IllegalArgumentException("explicit disposable lab scope required");
        }
        cgroupPrefix = "/ndjavaspike" + run + ".slice";
        home = Path.of("/var/tmp/nd-java-spike-" + run, "bundle");
        state = home.getParent().resolve("state");
        javaCommand = home.resolve("jdk/bin/java").toString();
    }

    record Target(String pid, String start, String uid, String gid) {
        String key() { return pid + ":" + start; }
    }

    static Target target(String pid) throws Exception {
        Path proc = Path.of("/proc", pid);
        // Check ownership before inspecting executable/argv, and again in the same-UID child.
        if (cgroupPrefix != null && !inCgroup(Files.readString(proc.resolve("cgroup")), cgroupPrefix)) return null;
        if (!Files.readSymbolicLink(proc.resolve("exe")).getFileName().toString().equals("java")) {
            return null;
        }
        // Deliberately fixture-scoped discovery: do not attach to monitor/helper JVMs.
        var argv = Arrays.asList(Files.readString(proc.resolve("cmdline")).split("\u0000"));
        int jar = argv.indexOf("-jar");
        if (jar < 0 || jar + 1 == argv.size() || !(argv.get(jar + 1).equals("/app/app.jar")
                || (cgroupPrefix != null && argv.get(jar + 1).equals(home.resolve("app.jar").toString())))) {
            return null;
        }
        String stat = Files.readString(proc.resolve("stat"));
        String start = stat.substring(stat.lastIndexOf(')') + 2).split(" ")[19];
        String uid = null, gid = null;
        for (String line : Files.readAllLines(proc.resolve("status"))) {
            if (line.startsWith("Uid:")) uid = line.trim().split("\\s+")[2];
            if (line.startsWith("Gid:")) gid = line.trim().split("\\s+")[2];
        }
        if (uid == null || gid == null) throw new IllegalStateException("missing credentials");
        return new Target(pid, start, uid, gid);
    }

    static void event(Target t, String state, String detail) {
        System.out.println("SCOUT\t" + t.key() + "\t" + state + "\t" + t.uid() + "\t"
                + detail.replace('\n', ' ').replace('\t', ' '));
        System.out.flush();
    }

    static void inject(String[] args) throws Exception {
        Target original = new Target(args[1], args[2], args[3], args[4]);
        if (!original.equals(target(original.pid()))) throw new IllegalStateException("identity changed");
        // This child has already dropped to the target UID/GID. Target /tmp is intentionally untrusted;
        // atomic fresh directory creation avoids adopting pre-existing paths. This is not a production helper.
        Path directory = Files.createTempDirectory(Path.of("/proc", original.pid(), "root", "tmp"),
                "netdata-java-spike-", PosixFilePermissions.asFileAttribute(PosixFilePermissions.fromString("rwx------")));
        Files.copy(home.resolve("otel.jar"), directory.resolve("otel.jar"));
        Files.copy(home.resolve("hikari-extension.jar"), directory.resolve("hikari-extension.jar"));
        if (!original.equals(target(original.pid()))) throw new IllegalStateException("identity changed after delivery");
        String visible = "/tmp/" + directory.getFileName();
        String options = args[5].replace("/lab/hikari-extension.jar", visible + "/hikari-extension.jar");
        var vm = VirtualMachine.attach(original.pid());
        try {
            vm.loadAgent(visible + "/otel.jar", options);
        } finally {
            vm.detach();
        }
        event(original, "loaded", visible);
    }

    public static void main(String[] args) throws Exception {
        if (args.length == 1 && args[0].equals("--self-test")) {
            String prefix = "/ndjavaspike012345abcdef.slice";
            if (!inCgroup("0::" + prefix + "/target.service\n", prefix)
                    || inCgroup("0::" + prefix + "-other/target.service\n", prefix)
                    || inCgroup("0::/system.slice/target.service\n", prefix)
                    || inCgroup("1::" + prefix + "/target.service\n", prefix)) {
                throw new AssertionError("cgroup boundary failure");
            }
            System.out.println("exact cgroup boundary checks passed");
            return;
        }
        configure();
        if (args.length > 0 && args[0].equals("inject")) {
            inject(args);
            return;
        }
        String base = System.getenv("SCOUT_OPTIONS");
        String run = System.getenv("SCOUT_RUN");
        if (base == null || run == null) throw new IllegalArgumentException("lab options/run required");
        Path journal = state.resolve("attempts");
        Set<String> attempted = new HashSet<>();
        Set<String> denied = new HashSet<>();
        boolean scanOnly = "true".equals(System.getenv("SCOUT_SCAN_ONCE"));
        if (Files.exists(journal)) attempted.addAll(Files.readAllLines(journal));
        while (true) {
            try (var paths = Files.list(Path.of("/proc"))) {
                for (Path proc : paths.filter(p -> p.getFileName().toString().matches("[0-9]+")).sorted().toList()) {
                    Target t;
                    try {
                        t = target(proc.getFileName().toString());
                    } catch (Exception error) {
                        if (cgroupPrefix != null && Files.exists(proc) && denied.size() < 64
                                && denied.add(proc.getFileName().toString())) {
                            System.out.println("SCAN_ACCESS\t" + proc.getFileName() + "\t" + error.getClass().getSimpleName());
                        }
                        continue;
                    }
                    if (t == null || attempted.contains(t.key())) continue;
                    if (scanOnly) {
                        event(t, "discovered", "read-only scope check");
                        continue;
                    }
                    if (attempted.size() >= 64) throw new IllegalStateException("lab attempt cap exceeded");
                    // Record before launch: monitor restart must not blindly retry uncertain attachment.
                    Files.writeString(journal, t.key() + "\n", StandardOpenOption.CREATE, StandardOpenOption.APPEND);
                    attempted.add(t.key());
                    event(t, "attempt", "java-app-" + t.uid());
                    String options = base + ";otel.service.name=java-app-" + t.uid()
                            + ";otel.resource.attributes=service.instance.id=" + run + "-" + t.key();
                    var child = new ProcessBuilder("setpriv", "--reuid=" + t.uid(), "--regid=" + t.gid(),
                            "--clear-groups", "--inh-caps=-all", "--ambient-caps=-all", "--no-new-privs",
                            javaCommand, "-Xms16m", "-Xmx64m", "-cp", home.toString(), "Scout", "inject", t.pid(),
                            t.start(), t.uid(), t.gid(), options).inheritIO().start();
                    if (!child.waitFor(30, TimeUnit.SECONDS)) {
                        child.destroyForcibly();
                        child.waitFor();
                        event(t, "timeout", "attachment outcome unknown; no automatic retry");
                    } else {
                        event(t, child.exitValue() == 0 ? "acknowledged" : "failed", "exit=" + child.exitValue());
                    }
                }
            }
            if (scanOnly) return;
            Thread.sleep(1000);
        }
    }
}
