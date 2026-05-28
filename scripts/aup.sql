CREATE ROLE "teleport-admin" WITH LOGIN CREATEROLE;
CREATE ROLE creator;
GRANT CREATE ON SCHEMA public TO creator;
GRANT creator TO "teleport-admin" WITH ADMIN OPTION;
CREATE ROLE bob WITH LOGIN;
GRANT bob TO "teleport-admin";

