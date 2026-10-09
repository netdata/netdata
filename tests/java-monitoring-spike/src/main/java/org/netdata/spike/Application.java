package org.netdata.spike;

import java.sql.Connection;
import java.util.Map;
import javax.sql.DataSource;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

// Ordinary application code: no Actuator, JMX, Micrometer, or monitoring hooks.
@SpringBootApplication
@RestController
public class Application {
    private final DataSource dataSource;

    public Application(DataSource dataSource) {
        this.dataSource = dataSource;
    }

    public static void main(String[] args) {
        SpringApplication.run(Application.class, args);
    }

    @GetMapping("/work")
    public ResponseEntity<?> work(@RequestParam(defaultValue = "false") boolean fail,
                                  @RequestParam(defaultValue = "5") int holdMs) throws Exception {
        try (Connection connection = dataSource.getConnection();
             var statement = connection.createStatement();
             var rows = statement.executeQuery("SELECT 42")) {
            rows.next();
            Thread.sleep(Math.max(0, Math.min(holdMs, 100)));
            return ResponseEntity.status(fail ? 503 : 200).body(Map.of("answer", rows.getInt(1)));
        }
    }
}
