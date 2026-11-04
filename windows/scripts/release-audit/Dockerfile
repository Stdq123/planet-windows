FROM node:26.12.1-bookworm
RUN dpkg --add-architecture i386 && apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends wine32 wine64 xvfb xauth ca-certificates \
    && rm -rf /var/lib/apt/lists/*
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends wine \
    && rm -rf /var/lib/apt/lists/*
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends gcc-mingw-w64-x86-64 \
    && rm -rf /var/lib/apt/lists/*
COPY bcryptprimitives.c /tmp/bcryptprimitives.c
RUN mkdir -p /opt/planet-wine-test \
    && x86_64-w64-mingw32-gcc -shared -O2 -o /opt/planet-wine-test/bcryptprimitives.dll /tmp/bcryptprimitives.c -lbcrypt \
    && rm /tmp/bcryptprimitives.c
ENV WINEDEBUG=-all WINEPREFIX=/tmp/planet-audit-wine HOME=/tmp/planet-audit-home
