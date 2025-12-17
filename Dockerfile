FROM golang:1.24.3

SHELL ["/bin/bash", "-c"]

RUN apt update && apt install -y unzip

RUN curl -fsSL https://bun.sh/install | bash

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Build
RUN source ~/.bashrc && make

ENV PWRQ_PORT=8084

ENTRYPOINT [ "/app/pwrq", "-i"]
