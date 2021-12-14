DROP TABLE IF EXISTS anomaly_rate_info_test_queried_0;
CREATE TABLE anomaly_rate_info_test_queried_0 (dim_id text NOT NULL, anomaly_percentage real);
INSERT INTO anomaly_rate_info_test_queried_0 (dim_id, anomaly_percentage)
SELECT  main.dimension_id, COALESCE(
     ((COALESCE(pre.avg,0) * (SELECT before-1638208343 FROM anomaly_rate_info_test_source_0 WHERE after < 1638208343 ORDER BY after DESC LIMIT 1)) + 
     (COALESCE(main.avg,0) * ((SELECT before FROM anomaly_rate_info_test_source_0 WHERE before <= 1638208583 ORDER BY before DESC LIMIT 1) - 
                  (SELECT after FROM anomaly_rate_info_test_source_0 WHERE after >= 1638208343 ORDER BY after ASC LIMIT 1))) + 
     (COALESCE(post.avg,0) * (SELECT 1638208583-after FROM anomaly_rate_info_test_source_0 WHERE before > 1638208583 OR ((SELECT before FROM anomaly_rate_info_test_source_0 
                                                                                                WHERE before <= 1638208583 
                                                                                                ORDER BY before DESC LIMIT 1) 
                                                                                            AND NOT EXISTS (SELECT before FROM anomaly_rate_info_test_source_0 
                                                                                                                WHERE before > 1638208583 
                                                                                                                ORDER BY before ASC LIMIT 1)) 
                                                                ORDER BY before ASC LIMIT 1))) / 
     ( 
     (SELECT before-1638208343 FROM anomaly_rate_info_test_source_0 WHERE after < 1638208343 ORDER BY after DESC LIMIT 1) + 
     (SELECT before FROM anomaly_rate_info_test_source_0 WHERE before <= 1638208583 ORDER BY before DESC LIMIT 1) - 
     (SELECT after FROM anomaly_rate_info_test_source_0 WHERE after >= 1638208343 ORDER BY after ASC LIMIT 1) + 
     (SELECT 1638208583-after FROM anomaly_rate_info_test_source_0 WHERE before > 1638208583 OR ((SELECT before FROM anomaly_rate_info_test_source_0 
                                                                                                WHERE before <= 1638208583 
                                                                                                ORDER BY before DESC LIMIT 1) 
                                                                                            AND NOT EXISTS (SELECT before FROM anomaly_rate_info_test_source_0 
                                                                                                                WHERE before > 1638208583 
                                                                                                                ORDER BY before ASC LIMIT 1)) 
                                                                ORDER BY before ASC LIMIT 1)
		),0.0) avg FROM 
    (SELECT dimension_id, AVG(anomaly_percentage) avg FROM 
      (SELECT json_extract(j.value, '$[1]') AS dimension_id, 
      json_extract(j.value, '$[0]') AS anomaly_percentage 
      FROM anomaly_rate_info_test_source_0 AS ari, json_each(ari.anomaly_rates) AS j 
      WHERE ari.host_id == '5cb28cec-3d65-11ec-83fd-15e0d7613f4b' AND ari.after >= 1638208343 AND ari.before <= 1638208583 
      AND json_valid(ari.anomaly_rates)) 
      GROUP BY dimension_id) AS main 
     LEFT JOIN 
    (SELECT dimension_id, AVG(anomaly_percentage) avg FROM 
      (SELECT json_extract(j.value, '$[1]') AS dimension_id, 
      json_extract(j.value, '$[0]') AS anomaly_percentage 
      FROM anomaly_rate_info_test_source_0 AS ari, json_each(ari.anomaly_rates) AS j 
      WHERE ari.host_id == '5cb28cec-3d65-11ec-83fd-15e0d7613f4b' 
      AND ari.after >= (SELECT after FROM anomaly_rate_info_test_source_0 WHERE after <= 1638208343 ORDER BY after DESC LIMIT 1) 
      AND ari.before <= (SELECT before FROM anomaly_rate_info_test_source_0 WHERE before >= 1638208343 ORDER BY before ASC LIMIT 1) 
      AND json_valid(ari.anomaly_rates)) 
      GROUP BY dimension_id) AS pre 
     ON main.dimension_id = pre.dimension_id 
     LEFT JOIN 
    (SELECT dimension_id, AVG(anomaly_percentage) avg FROM 
      (SELECT json_extract(j.value, '$[1]') AS dimension_id, 
      json_extract(j.value, '$[0]') AS anomaly_percentage 
      FROM anomaly_rate_info_test_source_0 AS ari, json_each(ari.anomaly_rates) AS j 
      WHERE ari.host_id == '5cb28cec-3d65-11ec-83fd-15e0d7613f4b' 
      AND ari.after >= (SELECT after FROM anomaly_rate_info_test_source_0 WHERE after <= 1638208583 ORDER BY after DESC LIMIT 1) 
      AND ari.before <= (SELECT before FROM anomaly_rate_info_test_source_0 WHERE before >= 1638208583 OR ((SELECT before FROM anomaly_rate_info_test_source_0 
                                                                                                WHERE before <= 1638208583 
                                                                                                ORDER BY before DESC LIMIT 1) 
                                                                                            AND NOT EXISTS (SELECT before FROM anomaly_rate_info_test_source_0 
                                                                                                            WHERE before >= 1638208583 
                                                                                                            ORDER BY before ASC LIMIT 1)) 
                                                                ORDER BY before ASC LIMIT 1) 
      AND json_valid(ari.anomaly_rates)) 
      GROUP BY dimension_id) AS post 
     ON main.dimension_id = post.dimension_id 
    GROUP BY main.dimension_id;