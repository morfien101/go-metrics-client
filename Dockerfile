FROM silverstagtech/distroless:latest

LABEL maintainer="Randy Coburn <morfien101@gmail.com>"

COPY ./artifacts/metrics-client /

ENTRYPOINT [ "/metrics-client" ]
CMD []