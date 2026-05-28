FROM redis:7.2.3

USER root

COPY redis.conf users.acl /usr/local/etc/redis/
COPY redis.cas /usr/local/etc/redis/certs/redis.cas
COPY redis.crt /usr/local/etc/redis/certs/redis.crt
COPY redis.key /usr/local/etc/redis/certs/redis.key
RUN chown -R redis:redis /usr/local/etc/redis/

USER redis

CMD [ "redis-server", "/usr/local/etc/redis/redis.conf" ]
