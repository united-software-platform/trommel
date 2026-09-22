# Образ Trommel: тулчейн живёт только в стадии сборки, в рантайм уезжает один статический
# бинарник. Проверяемый проект монтируется в этот образ снаружи и только на чтение —
# проверяльщик не имеет права править проверяемое.
ARG GO_IMAGE=golang:1.27.1
ARG RUNTIME_IMAGE=alpine:3

FROM ${GO_IMAGE} AS build

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd

# CGO отключён намеренно: статический бинарник не тянет в рантайм ни компоновщик,
# ни библиотеки C, и образ выполнения остаётся минимальным.
RUN CGO_ENABLED=0 go build -trimpath -o /out/trommel ./cmd/trommel

FROM ${RUNTIME_IMAGE}

# Выделенные непривилегированные пользователь и группа [DOCK-012]
RUN addgroup -g 1000 trommel \
    && adduser -u 1000 -G trommel -D -h /srv/trommel trommel

WORKDIR /srv/trommel

# Рабочий каталог и файлы принадлежат пользователю приложения [DOCK-013]
COPY --from=build --chown=trommel:trommel /out/trommel /usr/local/bin/trommel

# Процесс исполняется от непривилегированного пользователя [DOCK-012]
USER trommel

ENTRYPOINT ["trommel"]
