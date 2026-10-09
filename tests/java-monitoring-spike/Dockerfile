ARG RUNTIME_IMAGE=eclipse-temurin:21-jdk@sha256:3e3c176ffed168beb42c607be9bc1639b466cf00261a0fb04425562c9d0c5c2b
FROM maven:3.9.11-eclipse-temurin-17@sha256:e4a7ace3dc0d645ed97f8d9ad0b0d3f0b14fa8d150138f27f116d7105a639b82 AS build
WORKDIR /build
COPY pom.xml .
COPY src ./src
RUN mvn -B -ntp package
COPY extension ./extension
RUN mvn -B -ntp -f extension/pom.xml package
COPY lifecycle ./lifecycle
RUN mvn -B -ntp -f lifecycle/pom.xml package
COPY Probe.java .
RUN javac --release 17 Probe.java
ADD https://github.com/open-telemetry/opentelemetry-java-instrumentation/releases/download/v2.32.0/opentelemetry-javaagent.jar /build/otel.jar
RUN echo 'f787eb6c7f3d18e69a431e108a15278d25ee37f83d68b678f621e063f3988f82  /build/otel.jar' | sha256sum -c -

FROM ${RUNTIME_IMAGE}
WORKDIR /lab
COPY --from=build /build/target/java-monitoring-fixture-1.0.jar /lab/app.jar
COPY --from=build /build/Probe.class /lab/Probe.class
COPY --from=build /build/otel.jar /lab/otel.jar
COPY --from=build /build/extension/target/hikari-extension-1.0.jar /lab/hikari-extension.jar
COPY --from=build /build/lifecycle/target/pool-lifecycle-fixture-1.0.jar /lab/lifecycle.jar
COPY --from=build /build/lifecycle/target/dependency /lab/lib
RUN chmod 644 /lab/otel.jar
USER 10001:10001
ENTRYPOINT ["java", "-Xms128m", "-Xmx256m"]
CMD ["-jar", "/lab/app.jar"]
