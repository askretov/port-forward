ARG BUILDER_IMAGE=golang:1.21-alpine3.17
ARG RUNNER_IMAGE=alpine:3.17

#################### Build the project ####################
FROM $BUILDER_IMAGE AS builder

RUN apk --update add git make openssh
#RUN addgroup -g 1001 -S builder && \
#    adduser -u 1001 -S builder -G builder
#
#USER builder:builder


ARG BUILD_DIR=/app
COPY . $BUILD_DIR/
WORKDIR $BUILD_DIR
RUN pwd
RUN ls -la
#RUN git config --global --add safe.directory $BUILD_DIR
RUN make build

#################### Build a runner image ####################
FROM $RUNNER_IMAGE

ARG BUILD_DIR=/app
ENV APP_HOME=/app/bin
ENV BINARY=app

#RUN addgroup -g 1001 -S runner && \
#    adduser -u 1001 -S runner -G runner
#USER runner:runner
COPY --from=builder $BUILD_DIR/bin/ $APP_HOME

WORKDIR $APP_HOME
ENTRYPOINT $APP_HOME/$BINARY
