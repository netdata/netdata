-- pg_stat_monitor initialization script
CREATE EXTENSION IF NOT EXISTS pg_stat_monitor;

CREATE TABLE IF NOT EXISTS public.sample (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    value INTEGER NOT NULL
);

INSERT INTO public.sample (name, value)
VALUES
    ('alpha', 10),
    ('beta', 20),
    ('gamma', 30);

SELECT COUNT(*) FROM public.sample;
SELECT * FROM public.sample WHERE value > 15;
UPDATE public.sample SET value = value + 1 WHERE name = 'alpha';
