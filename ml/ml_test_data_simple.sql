DROP TABLE IF EXISTS anomaly_rate_info_test_source_simple;
CREATE TABLE anomaly_rate_info_test_source_simple (host_id text NOT NULL, after int NOT NULL, before int NOT NULL, anomaly_rates text);
INSERT INTO anomaly_rate_info_test_source_simple (host_id, after, before, anomaly_rates) VALUES
(
'aaa',	10,	20,	'[
    [
        0.0,
        "system.idlejitter|min"
    ],
    [
        0.0,
        "system.idlejitter|max"
    ]
]'
),(
'aaa',	20,	30,	'[
    [
        0.0,
        "system.idlejitter|min"
    ],
    [
        5.0,
        "system.idlejitter|max"
    ]
]'
),(
'aaa',	30,	40,	'[
    [
        0.0,
        "system.idlejitter|min"
    ],
    [
        0.0,
        "system.idlejitter|max"
    ]
]'
),(
'aaa',	40,	50,	'[
    [
        0.0,
        "system.idlejitter|min"
    ],
    [
        0.0,
        "system.idlejitter|max"
    ]
]'
),(
'aaa',	50,	60,	'[
    [
        0.0,
        "system.idlejitter|min"
    ],
    [
        0.0,
        "system.idlejitter|max"
    ]
]'
);

DROP TABLE IF EXISTS anomaly_rate_info_test_results_simple;
CREATE TABLE anomaly_rate_info_test_results_simple (dim_id text NOT NULL, anomaly_percentage real);
INSERT INTO anomaly_rate_info_test_results_simple (dim_id, anomaly_percentage) VALUES
("system.idlejitter|min", 0.0),
("system.idlejitter|max", 1.7);