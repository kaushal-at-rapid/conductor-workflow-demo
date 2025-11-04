FROM eclipse-temurin:17-jre
WORKDIR /app
ADD https://repo1.maven.org/maven2/com/netflix/conductor/conductor-server/3.15.0/conductor-server-3.15.0-boot.jar conductor-server.jar
EXPOSE 8080 5000
ENTRYPOINT ["java", "-jar", "conductor-server.jar"]
