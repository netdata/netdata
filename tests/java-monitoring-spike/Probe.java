import com.sun.tools.attach.VirtualMachine;
import java.util.TreeSet;
import javax.management.MBeanServerConnection;
import javax.management.ObjectName;
import javax.management.remote.JMXConnectorFactory;
import javax.management.remote.JMXServiceURL;

// Lab-only explicit PID probe. It never discovers or attaches to unrelated processes.
public class Probe {
    public static void main(String[] args) throws Exception {
        var vm = VirtualMachine.attach(args[1]);
        try {
            if (args[0].equals("agent")) {
                vm.loadAgent(args[2], args[3]);
                System.out.println("loadAgent returned successfully; verify telemetry separately");
                return;
            }
            String address = vm.startLocalManagementAgent();
            try (var connector = JMXConnectorFactory.connect(new JMXServiceURL(address))) {
                MBeanServerConnection server = connector.getMBeanServerConnection();
                for (ObjectName name : new TreeSet<>(server.queryNames(null, null))) {
                    System.out.println("MBEAN " + name.getCanonicalName());
                }
                for (String bean : new String[]{"java.lang:type=Memory", "java.lang:type=Threading",
                        "java.lang:type=OperatingSystem"}) {
                    ObjectName name = new ObjectName(bean);
                    for (var attribute : server.getMBeanInfo(name).getAttributes()) {
                        if (attribute.isReadable()) {
                            try {
                                System.out.println("VALUE " + bean + " " + attribute.getName() + "="
                                        + server.getAttribute(name, attribute.getName()));
                            } catch (Exception e) {
                                System.out.println("UNAVAILABLE " + bean + " " + attribute.getName()
                                        + " " + e.getClass().getSimpleName());
                            }
                        }
                    }
                }
            }
        } finally {
            vm.detach();
        }
    }
}
