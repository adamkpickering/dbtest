FROM ubuntu:24.04

# Installing curl is critical because it sets up a directory we
# later use for certificates
RUN apt-get update && apt-get install -y curl

COPY --from=mkcertca rootCA.pem /usr/local/share/ca-certificates/mkcertCA.crt
RUN update-ca-certificates

COPY --from=bins teleport /usr/local/bin/teleport
COPY --from=bins tsh /usr/local/bin/tsh
COPY --from=bins tctl /usr/local/bin/tctl
COPY teleport.yaml /etc/teleport.yaml

CMD ["teleport", "start"]
