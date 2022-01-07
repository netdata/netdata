DROP TABLE IF EXISTS anomaly_rate_info_test_queried_3;
CREATE TABLE anomaly_rate_info_test_queried_3 (dim_id text NOT NULL, anomaly_percentage real);
INSERT INTO anomaly_rate_info_test_queried_3 (dim_id, anomaly_percentage)
SELECT dimension_id, SUM(delta*avg)/SUM(delta) avg FROM 
(
SELECT delta, dimension_id, AVG(anomaly_percentage) avg FROM 
      (SELECT before-after delta, json_extract(j.value, '$[1]') AS dimension_id, 
      json_extract(j.value, '$[0]') AS anomaly_percentage 
      FROM anomaly_rate_info_test_source AS ari, json_each(ari.anomaly_rates) AS j 
      WHERE ari.host_id == '5cb28cec-3d65-11ec-83fd-15e0d7613f4b' AND ari.after >= 1638208781 AND ari.before <= 1638208800 
      AND json_valid(ari.anomaly_rates)) 
      GROUP BY dimension_id
UNION 
SELECT delta, dimension_id, AVG(anomaly_percentage) avg FROM 
      (SELECT before-1638208781 delta, json_extract(j.value, '$[1]') AS dimension_id, 
      json_extract(j.value, '$[0]') AS anomaly_percentage 
      FROM anomaly_rate_info_test_source AS ari, json_each(ari.anomaly_rates) AS j 
      WHERE ari.host_id == '5cb28cec-3d65-11ec-83fd-15e0d7613f4b' 
      AND ari.after >= (SELECT after FROM anomaly_rate_info_test_source WHERE after <= 1638208781 ORDER BY after DESC LIMIT 1) 
      AND ari.before <= (SELECT before FROM anomaly_rate_info_test_source WHERE before >= 1638208781 ORDER BY before ASC LIMIT 1) 
      AND json_valid(ari.anomaly_rates)) 
      GROUP BY dimension_id
UNION
SELECT delta, dimension_id, AVG(anomaly_percentage) avg FROM 
      (SELECT 1638208800-after delta, json_extract(j.value, '$[1]') AS dimension_id, 
      json_extract(j.value, '$[0]') AS anomaly_percentage 
      FROM anomaly_rate_info_test_source AS ari, json_each(ari.anomaly_rates) AS j 
      WHERE ari.host_id == '5cb28cec-3d65-11ec-83fd-15e0d7613f4b' 
      AND ari.after >= (SELECT after FROM anomaly_rate_info_test_source WHERE after <= 1638208800 ORDER BY after DESC LIMIT 1) 
      AND ari.before <= (SELECT before FROM anomaly_rate_info_test_source WHERE before >= 1638208800 OR ((SELECT before FROM anomaly_rate_info_test_source 
                                                                                                WHERE before <= 1638208800 
                                                                                                ORDER BY before DESC LIMIT 1) 
                                                                                            AND NOT EXISTS (SELECT before FROM anomaly_rate_info_test_source 
                                                                                                            WHERE before >= 1638208800 
                                                                                                            ORDER BY before ASC LIMIT 1)) 
                                                                ORDER BY before ASC LIMIT 1) 
      AND json_valid(ari.anomaly_rates)) 
      GROUP BY dimension_id
)
GROUP BY dimension_id;