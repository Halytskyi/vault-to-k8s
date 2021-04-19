FROM alpine:3.7

RUN apk update && \
    apk --no-cache add ca-certificates git && \
    update-ca-certificates

COPY ./app /app
ENTRYPOINT /app
