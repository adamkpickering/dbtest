FROM redis:7.2.3

USER root

COPY redis.conf users.acl /usr/local/etc/redis/
COPY combined.crt /usr/local/etc/redis/certs/combined.crt
COPY node.crt /usr/local/etc/redis/certs/node.crt
COPY node.key /usr/local/etc/redis/certs/node.key
RUN chown -R redis:redis /usr/local/etc/redis/

USER redis

CMD [ "redis-server", "/usr/local/etc/redis/redis.conf" ]
