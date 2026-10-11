package org.netdata.spike.hikari;

import static net.bytebuddy.matcher.ElementMatchers.named;
import static net.bytebuddy.matcher.ElementMatchers.takesArguments;

import com.zaxxer.hikari.HikariDataSource;
import io.opentelemetry.javaagent.extension.instrumentation.InstrumentationModule;
import io.opentelemetry.javaagent.extension.instrumentation.TypeInstrumentation;
import io.opentelemetry.javaagent.extension.instrumentation.TypeTransformer;
import java.util.List;
import net.bytebuddy.asm.Advice;
import net.bytebuddy.description.type.TypeDescription;
import net.bytebuddy.matcher.ElementMatcher;

public class HikariModule extends InstrumentationModule {
    public HikariModule() {
        super("netdata-hikari");
    }

    @Override
    public List<TypeInstrumentation> typeInstrumentations() {
        return List.of(new DataSourceInstrumentation());
    }

    @Override
    public List<String> getAdditionalHelperClassNames() {
        return List.of("org.netdata.spike.hikari.PoolObserver",
                       "org.netdata.spike.hikari.PoolObserver$Registration");
    }

    public static class DataSourceInstrumentation implements TypeInstrumentation {
        @Override
        public ElementMatcher<TypeDescription> typeMatcher() {
            return named("com.zaxxer.hikari.HikariDataSource");
        }

        @Override
        public void transform(TypeTransformer transformer) {
            transformer.applyAdviceToMethod(named("getConnection").and(takesArguments(0)),
                    HikariModule.class.getName() + "$BorrowAdvice");
            transformer.applyAdviceToMethod(named("close").and(takesArguments(0)),
                    HikariModule.class.getName() + "$CloseAdvice");
        }
    }

    public static class BorrowAdvice {
        @Advice.OnMethodEnter(suppress = Throwable.class)
        public static boolean enter(@Advice.This HikariDataSource source) {
            return PoolObserver.observe(source);
        }

        @Advice.OnMethodExit(onThrowable = Throwable.class, suppress = Throwable.class)
        public static void exit(@Advice.This HikariDataSource source, @Advice.Enter boolean observed) {
            // A lazy pool may only become available during this first borrow.
            if (!observed) {
                PoolObserver.observe(source);
            }
        }
    }

    public static class CloseAdvice {
        @Advice.OnMethodExit(onThrowable = Throwable.class, suppress = Throwable.class)
        public static void exit(@Advice.This HikariDataSource source) {
            PoolObserver.remove(source);
        }
    }
}
