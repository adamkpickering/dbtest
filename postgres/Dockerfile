FROM postgres:18

ENV POSTGRES_PASSWORD=password

COPY postgres.cas /pg-certs/postgres.cas
COPY postgres.crt /pg-certs/postgres.crt
COPY postgres.key /pg-certs/postgres.key
RUN chown postgres:postgres /pg-certs/postgres.cas /pg-certs/postgres.crt /pg-certs/postgres.key \
    && chmod 600 /pg-certs/postgres.key

COPY pg_hba.conf /etc/postgresql/pg_hba.conf

EXPOSE 5432
