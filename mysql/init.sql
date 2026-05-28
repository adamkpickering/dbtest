CREATE DATABASE `teleport`;

CREATE USER 'teleport-admin'@'%' REQUIRE SUBJECT '/CN=teleport-admin';
GRANT ALL PRIVILEGES ON *.* TO 'teleport-admin'@'%' WITH GRANT OPTION;
-- GRANT ALTER ROUTINE, CREATE ROUTINE, EXECUTE ON `teleport`.* TO 'teleport-admin';

CREATE DATABASE public;

CREATE ROLE "creator";
GRANT CREATE ON `public`.* TO 'creator';
GRANT SELECT ON `public`.* TO 'creator';

CREATE TABLE public.example_table (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);
