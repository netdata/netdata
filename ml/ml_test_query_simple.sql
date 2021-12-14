DROP TABLE IF EXISTS anomaly_rate_info_test_queried_simple;
CREATE TABLE anomaly_rate_info_test_queried_simple (dim_id text NOT NULL, anomaly_percentage real);
INSERT INTO anomaly_rate_info_test_queried_simple (dim_id, anomaly_percentage)
SELECT  main.dimension_id, COALESCE(
     ((COALESCE(pre.avg,0) * (SELECT before-15 FROM anomaly_rate_info_test_source_simple WHERE after < 15 ORDER BY after DESC LIMIT 1)) + 
     (COALESCE(main.avg,0) * ((SELECT before FROM anomaly_rate_info_test_source_simple WHERE before <= 45 ORDER BY before DESC LIMIT 1) - 
                  (SELECT after FROM anomaly_rate_info_test_source_simple WHERE after >= 15 ORDER BY after ASC LIMIT 1))) + 
     (COALESCE(post.avg,0) * (SELECT 45-after FROM anomaly_rate_info_test_source_simple WHERE before > 45 OR ((SELECT before FROM anomaly_rate_info_test_source_simple 
                                                                                                WHERE before <= 45 
                                                                                                ORDER BY before DESC LIMIT 1) 
                                                                                            AND NOT EXISTS (SELECT before FROM anomaly_rate_info_test_source_simple 
                                                                                                                WHERE before > 45 
                                                                                                                ORDER BY before ASC LIMIT 1)) 
                                                                ORDER BY before ASC LIMIT 1))) / 
     ( 
     (SELECT before-15 FROM anomaly_rate_info_test_source_simple WHERE after < 15 ORDER BY after DESC LIMIT 1) + 
     (SELECT before FROM anomaly_rate_info_test_source_simple WHERE before <= 45 ORDER BY before DESC LIMIT 1) - 
     (SELECT after FROM anomaly_rate_info_test_source_simple WHERE after >= 15 ORDER BY after ASC LIMIT 1) + 
     (SELECT 45-after FROM anomaly_rate_info_test_source_simple WHERE before > 45 OR ((SELECT before FROM anomaly_rate_info_test_source_simple 
                                                                                                WHERE before <= 45 
                                                                                                ORDER BY before DESC LIMIT 1) 
                                                                                            AND NOT EXISTS (SELECT before FROM anomaly_rate_info_test_source_simple 
                                                                                                                WHERE before > 45 
                                                                                                                ORDER BY before ASC LIMIT 1)) 
                                                                ORDER BY before ASC LIMIT 1)
		),0.0) avg FROM 
    (SELECT dimension_id, AVG(anomaly_percentage) avg FROM 
      (SELECT json_extract(j.value, '$[1]') AS dimension_id, 
      json_extract(j.value, '$[0]') AS anomaly_percentage 
      FROM anomaly_rate_info_test_source_simple AS ari, json_each(ari.anomaly_rates) AS j 
      WHERE ari.host_id == 'aaa' AND ari.after >= 15 AND ari.before <= 45 
      AND json_valid(ari.anomaly_rates)) 
      GROUP BY dimension_id) AS main 
     LEFT JOIN 
    (SELECT dimension_id, AVG(anomaly_percentage) avg FROM 
      (SELECT json_extract(j.value, '$[1]') AS dimension_id, 
      json_extract(j.value, '$[0]') AS anomaly_percentage 
      FROM anomaly_rate_info_test_source_simple AS ari, json_each(ari.anomaly_rates) AS j 
      WHERE ari.host_id == 'aaa' 
      AND ari.after >= (SELECT after FROM anomaly_rate_info_test_source_simple WHERE after <= 15 ORDER BY after DESC LIMIT 1) 
      AND ari.before <= (SELECT before FROM anomaly_rate_info_test_source_simple WHERE before >= 15 ORDER BY before ASC LIMIT 1) 
      AND json_valid(ari.anomaly_rates)) 
      GROUP BY dimension_id) AS pre 
     ON main.dimension_id = pre.dimension_id 
     LEFT JOIN 
    (SELECT dimension_id, AVG(anomaly_percentage) avg FROM 
      (SELECT json_extract(j.value, '$[1]') AS dimension_id, 
      json_extract(j.value, '$[0]') AS anomaly_percentage 
      FROM anomaly_rate_info_test_source_simple AS ari, json_each(ari.anomaly_rates) AS j 
      WHERE ari.host_id == 'aaa' 
      AND ari.after >= (SELECT after FROM anomaly_rate_info_test_source_simple WHERE after <= 45 ORDER BY after DESC LIMIT 1) 
      AND ari.before <= (SELECT before FROM anomaly_rate_info_test_source_simple WHERE before >= 45 OR ((SELECT before FROM anomaly_rate_info_test_source_simple 
                                                                                                WHERE before <= 45 
                                                                                                ORDER BY before DESC LIMIT 1) 
                                                                                            AND NOT EXISTS (SELECT before FROM anomaly_rate_info_test_source_simple 
                                                                                                            WHERE before >= 45 
                                                                                                            ORDER BY before ASC LIMIT 1)) 
                                                                ORDER BY before ASC LIMIT 1) 
      AND json_valid(ari.anomaly_rates)) 
      GROUP BY dimension_id) AS post 
     ON main.dimension_id = post.dimension_id 
    GROUP BY main.dimension_id;