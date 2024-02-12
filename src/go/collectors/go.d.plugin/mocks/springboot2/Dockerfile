FROM gradle:5.0-jdk8-alpine as builder

COPY --chown=gradle:gradle . /home/gradle/src
WORKDIR /home/gradle/src
RUN gradle build

FROM openjdk:8-jre-alpine

EXPOSE 8080
COPY --from=builder /home/gradle/src/build/libs/springboot2-0.1.0.jar /app/

CMD ["java", "-jar", "/app/springboot2-0.1.0.jar"]