# Use the official MySQL image as the base image
FROM mariadb:11

COPY mariadb.cas /certs/mariadb.cas
COPY mariadb.crt /certs/mariadb.crt
COPY mariadb.key /certs/mariadb.key
COPY my.cnf /etc/mysql/conf.d/my.cnf
COPY init.sql /docker-entrypoint-initdb.d/init.sql
RUN chown -R mysql:mysql \
    /certs \
    /etc/mysql/conf.d/my.cnf \
    /docker-entrypoint-initdb.d

# Expose the MySQL port
EXPOSE 3306

ENV MARIADB_ROOT_PASSWORD=root
