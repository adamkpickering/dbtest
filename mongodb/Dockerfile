FROM mongo:5

COPY mongodb.cas /certs/mongodb.cas
COPY mongodb.crt /certs/mongodb.crt
COPY mongod.conf /etc/mongo/mongod.conf
COPY init.js /docker-entrypoint-initdb.d/init.js
RUN chown -R mongodb:mongodb \
    /certs \
    /etc/mongo/mongod.conf \
    /docker-entrypoint-initdb.d

EXPOSE 27017

CMD [ "--config", "/etc/mongo/mongod.conf" ]
