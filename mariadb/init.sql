CREATE DATABASE `teleport`;
CREATE DATABASE `public`;

CREATE USER 'teleport-admin'@'%' REQUIRE SUBJECT '/CN=teleport-admin';
GRANT PROCESS, CREATE USER ON *.* TO 'teleport-admin';
GRANT SELECT ON mysql.roles_mapping TO 'teleport-admin';
GRANT UPDATE ON mysql.* TO 'teleport-admin';
GRANT SELECT ON *.* TO 'teleport-admin';
GRANT ALL ON `teleport`.* TO 'teleport-admin' WITH GRANT OPTION;

CREATE ROLE creator WITH ADMIN 'teleport-admin';
GRANT CREATE ON `public`.* TO creator;
GRANT SELECT ON `public`.* TO creator;

CREATE TABLE public.example_table (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);
