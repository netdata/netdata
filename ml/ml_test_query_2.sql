DROP TABLE IF EXISTS anomaly_rate_info_test_queried_2;
CREATE TABLE anomaly_rate_info_test_queried_2 (dim_id text NOT NULL, anomaly_percentage real);
INSERT INTO anomaly_rate_info_test_queried_2 (dim_id, anomaly_percentage)
SELECT  main.dimension_id, COALESCE(
     ((COALESCE(pre.avg,0) * (SELECT before-after FROM anomaly_rate_info_test_source WHERE after <= 1638208643 ORDER BY after DESC LIMIT 1)) + 
     (COALESCE(main.avg,0) * ((SELECT before FROM anomaly_rate_info_test_source WHERE before <= 1638209003 ORDER BY before DESC LIMIT 1) - 
                  (SELECT after FROM anomaly_rate_info_test_source WHERE after >= 1638208643 ORDER BY after ASC LIMIT 1))) + 
     (COALESCE(post.avg,0) * (SELECT before-after FROM anomaly_rate_info_test_source WHERE before >= 1638209003 OR ((SELECT before FROM anomaly_rate_info_test_source 
                                                                                                WHERE before <= 1638209003 
                                                                                                ORDER BY before DESC LIMIT 1) 
                                                                                            AND NOT EXISTS (SELECT before FROM anomaly_rate_info_test_source 
                                                                                                                WHERE before >= 1638209003 
                                                                                                                ORDER BY before ASC LIMIT 1)) 
                                                                ORDER BY before ASC LIMIT 1))) / 
     ( 
     ((COALESCE(pre.avg,0)/(CASE WHEN COALESCE(pre.avg,1) = 0 THEN 0.0000001 ELSE COALESCE(pre.avg,1) END)
      * (SELECT before-after FROM anomaly_rate_info_test_source WHERE after <= 1638208643 ORDER BY after DESC LIMIT 1)) + 
     (COALESCE(main.avg,0)/(CASE WHEN COALESCE(main.avg,1) = 0 THEN 0.0000001 ELSE COALESCE(main.avg,1) END)
      * ((SELECT before FROM anomaly_rate_info_test_source WHERE before <= 1638209003 ORDER BY before DESC LIMIT 1) - 
                  (SELECT after FROM anomaly_rate_info_test_source WHERE after >= 1638208643 ORDER BY after ASC LIMIT 1))) + 
     (COALESCE(post.avg,0)/(CASE WHEN COALESCE(post.avg,1) = 0 THEN 0.0000001 ELSE COALESCE(post.avg,1) END)
      * (SELECT before-after FROM anomaly_rate_info_test_source WHERE before >= 1638209003 OR ((SELECT before FROM anomaly_rate_info_test_source 
                                                                                                WHERE before <= 1638209003 
                                                                                                ORDER BY before DESC LIMIT 1) 
                                                                                            AND NOT EXISTS (SELECT before FROM anomaly_rate_info_test_source 
                                                                                                                WHERE before >= 1638209003 
                                                                                                                ORDER BY before ASC LIMIT 1)) 
                                                                ORDER BY before ASC LIMIT 1)))
		),0.0) avg FROM 
    (SELECT dimension_id, AVG(anomaly_percentage) avg FROM 
      (SELECT json_extract(j.value, '$[1]') AS dimension_id, 
      json_extract(j.value, '$[0]') AS anomaly_percentage 
      FROM anomaly_rate_info_test_source AS ari, json_each(ari.anomaly_rates) AS j 
      WHERE ari.host_id == '5cb28cec-3d65-11ec-83fd-15e0d7613f4b' AND ari.after >= 1638208643 AND ari.before <= 1638209003 
      AND json_valid(ari.anomaly_rates)) 
      GROUP BY dimension_id) AS main 
     LEFT JOIN 
    (SELECT dimension_id, AVG(anomaly_percentage) avg FROM 
      (SELECT json_extract(j.value, '$[1]') AS dimension_id, 
      json_extract(j.value, '$[0]') AS anomaly_percentage 
      FROM anomaly_rate_info_test_source AS ari, json_each(ari.anomaly_rates) AS j 
      WHERE ari.host_id == '5cb28cec-3d65-11ec-83fd-15e0d7613f4b' 
      AND ari.after >= (SELECT after FROM anomaly_rate_info_test_source WHERE after <= 1638208643 ORDER BY after DESC LIMIT 1) 
      AND ari.before <= (SELECT before FROM anomaly_rate_info_test_source WHERE before >= 1638208643 ORDER BY before ASC LIMIT 1) 
      AND json_valid(ari.anomaly_rates)) 
      GROUP BY dimension_id) AS pre 
     ON main.dimension_id = pre.dimension_id 
     LEFT JOIN 
    (SELECT dimension_id, AVG(anomaly_percentage) avg FROM 
      (SELECT json_extract(j.value, '$[1]') AS dimension_id, 
      json_extract(j.value, '$[0]') AS anomaly_percentage 
      FROM anomaly_rate_info_test_source AS ari, json_each(ari.anomaly_rates) AS j 
      WHERE ari.host_id == '5cb28cec-3d65-11ec-83fd-15e0d7613f4b' 
      AND ari.after >= (SELECT after FROM anomaly_rate_info_test_source WHERE after <= 1638209003 ORDER BY after DESC LIMIT 1) 
      AND ari.before <= (SELECT before FROM anomaly_rate_info_test_source WHERE before >= 1638209003 OR ((SELECT before FROM anomaly_rate_info_test_source 
                                                                                                WHERE before <= 1638209003 
                                                                                                ORDER BY before DESC LIMIT 1) 
                                                                                            AND NOT EXISTS (SELECT before FROM anomaly_rate_info_test_source 
                                                                                                            WHERE before >= 1638209003 
                                                                                                            ORDER BY before ASC LIMIT 1)) 
                                                                ORDER BY before DESC LIMIT 1) 
      AND json_valid(ari.anomaly_rates)) 
      GROUP BY dimension_id) AS post 
     ON main.dimension_id = post.dimension_id 
    GROUP BY main.dimension_id;
