FROM ghcr.io/lavalink-devs/lavalink:4

USER root
COPY application.yml /opt/Lavalink/application.yml
RUN chmod 644 /opt/Lavalink/application.yml && \
    mkdir -p /opt/Lavalink/plugins && \
    chown -R lavalink:lavalink /opt/Lavalink
USER lavalink

EXPOSE 2333
