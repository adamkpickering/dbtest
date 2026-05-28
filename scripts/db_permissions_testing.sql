-- Create tables
CREATE TABLE allowed_test_table (
  id SERIAL PRIMARY KEY,
  value TEXT
);

CREATE TABLE disallowed_test_table (
  id SERIAL PRIMARY KEY,
  value TEXT
);

-- Populate tables with example data
INSERT INTO allowed_test_table (value) VALUES
  ('allowed_row_1'),
  ('allowed_row_2'),
  ('allowed_row_3');

INSERT INTO disallowed_test_table (value) VALUES
  ('disallowed_row_1'),
  ('disallowed_row_2'),
  ('disallowed_row_3');

-- Create views
CREATE VIEW allowed_test_view AS
SELECT id, value
FROM allowed_test_table;

CREATE VIEW disallowed_test_view AS
SELECT id, value
FROM disallowed_test_table;
