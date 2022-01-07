SELECT COUNT(r.dim_id) Count_of_incorrect_answers
    FROM 
    anomaly_rate_info_test_results_5 r 
    INNER JOIN
    anomaly_rate_info_test_queried_5 q 
    ON r.dim_id = q.dim_id
    WHERE ABS(r.anomaly_percentage - q.anomaly_percentage) > 0.1;