SELECT COUNT(r.dim_id) Count_of_incorrect_answers
    FROM 
    anomaly_rate_info_test_results_0 r 
    INNER JOIN
    anomaly_rate_info_test_queried_0 q 
    ON r.dim_id = q.dim_id
    WHERE ABS(r.anomaly_percentage - q.anomaly_percentage) > 0.5;