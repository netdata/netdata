# Java Plugin

## Code Style

- Stick to the configured code style
  - Eclipse formatter definition: `eclipse-formatter.xml`
  - To validate the code base run `./mvnw formatter:validate`
  - To reformat the code base run `./mvnw formattser:format`
- Stick to the configured import order
  - Import order configuration of plugin `net.revelc.code:impsort-maven-plugin` in `pom.xml`
  - To validate import order run `./mvnw impsort:check`
  - To organize imports run `./mvnw impsort:validate`