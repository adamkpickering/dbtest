# Use the official MySQL image as the base image
FROM mysql:8.3

COPY mysql.cas /certs/mysql.cas
COPY mysql.crt /certs/mysql.crt
COPY mysql.key /certs/mysql.key
COPY my.cnf /etc/mysql/conf.d/my.cnf
COPY init.sql /docker-entrypoint-initdb.d/init.sql
RUN chown -R mysql:mysql \
    /certs \
    /etc/mysql/conf.d/my.cnf \
    /docker-entrypoint-initdb.d

# Expose the MySQL port
EXPOSE 3306

ENV MYSQL_ROOT_PASSWORD=root
CMD [ "--default-authentication-plugin=mysql_native_password" ]
