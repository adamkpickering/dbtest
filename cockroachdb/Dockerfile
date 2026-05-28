FROM cockroachdb/cockroach:v26.1.1

COPY --chmod=600 certs /certs/certs

# root won't be able to connect unless we provide a client.root.[crt|key].
# Therefore, we create a custom client CA using `cockroach cert create-client-ca`.
# That command, with --overwrite, will prepend the custom client CA to client-ca.crt.
# This way, root can auth and init will succeed.
RUN /cockroach/cockroach cert create-client-ca \
    --certs-dir=/certs/certs \
    --ca-key=/certs/certs/ca-client.key \
    --overwrite

# make client.root.[crt|key]
RUN /cockroach/cockroach cert create-client root \
    --certs-dir=/certs/certs \
    --ca-key=/certs/certs/ca-client.key

# make client.node.[crt|key]
RUN /cockroach/cockroach cert create-client node \
    --certs-dir=/certs/certs \
    --ca-key=/certs/certs/ca-client.key

EXPOSE 26257

CMD [ "start-single-node", "--certs-dir=/certs/certs" ]
