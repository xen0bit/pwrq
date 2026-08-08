FROM golang:1.24.3

SHELL ["/bin/bash", "-c"]

RUN apt update && apt install -y unzip

RUN curl -fsSL https://bun.sh/install | bash

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# The image serves the browser IDE, so it needs the viz build.
RUN source ~/.bashrc && make build-viz-with-ide

ENV PWRQ_PORT=8084

ENTRYPOINT [ "/app/pwrq-viz", "-i"]
