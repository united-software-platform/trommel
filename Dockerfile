# Образ Trommel: тулчейны живут только в стадиях сборки, в рантайм уезжают два исполняемых
# файла — харнес и внешний анализатор кода. Проверяемый проект монтируется в этот образ
# снаружи и только на чтение — проверяльщик не имеет права править проверяемое.
ARG GO_IMAGE=golang:1.27.1
ARG FETCH_IMAGE=alpine:3
# База выполнения — glibc: выпуски анализатора собраны под gnu, сборок под musl у него нет,
# и в образе на основе Alpine его бинарник не запускается.
ARG RUNTIME_IMAGE=debian:12-slim

FROM ${GO_IMAGE} AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal

# CGO отключён намеренно: статический бинарник не тянет в рантайм ни компоновщик,
# ни библиотеки C, и образ выполнения остаётся минимальным.
RUN CGO_ENABLED=0 go build -trimpath -o /out/trommel ./cmd/trommel

# Стадия получения анализатора: выпуск скачивается и сверяется по контрольной сумме здесь,
# чтобы в образ выполнения уехал только проверенный бинарник, без сети и без загрузчика.
FROM ${FETCH_IMAGE} AS analyzer

ARG TLDR_VERSION=v0.4.0
ARG TLDR_ASSET=tldr-cli-x86_64-unknown-linux-gnu.tar.xz
ARG TLDR_SHA256=1455914111af163270dce630dd6c0293805c4a482c90689bd74bd9db49fb80bb
ARG TLDR_RELEASES=https://github.com/parcadei/tldr-code/releases/download

WORKDIR /fetch

# Сумма сверяется до распаковки: расхождение валит сборку образа, а не проявляется
# в вердикте. Из архива берётся только tldr — tldr-daemon и tldr-mcp прогону не нужны.
#
# Запуск анализатора здесь невозможен: стадия получения стоит на musl, а бинарник собран
# под gnu. Работоспособность проверяется в образе выполнения целью trommel-image-verify.
RUN apk add --no-cache ca-certificates wget xz \
    && wget -q "${TLDR_RELEASES}/${TLDR_VERSION}/${TLDR_ASSET}" \
    && echo "${TLDR_SHA256}  ${TLDR_ASSET}" | sha256sum -c - \
    && tar -xf "${TLDR_ASSET}" \
    && install -D -m 0755 "$(basename "${TLDR_ASSET}" .tar.xz)/tldr" /out/tldr

FROM ${RUNTIME_IMAGE}

# Выделенные непривилегированные пользователь и группа [DOCK-012]
RUN groupadd -g 1000 trommel \
    && useradd -u 1000 -g trommel -m -d /srv/trommel -s /bin/sh trommel

WORKDIR /srv/trommel

# Рабочий каталог и файлы принадлежат пользователю приложения [DOCK-013]
COPY --from=build --chown=trommel:trommel /out/trommel /usr/local/bin/trommel
COPY --from=analyzer --chown=trommel:trommel /out/tldr /usr/local/bin/tldr

# Процесс исполняется от непривилегированного пользователя [DOCK-012]
USER trommel

ENTRYPOINT ["trommel"]
