.PHONY: help init init-host init-env init-dirs init-gitignore init-ssh-key init-ssh-config \
	openspec-init trommel-tools trommel-toolchain trommel-build trommel-test \
	trommel-image trommel-image-verify trommel-deps trommel-fmt trommel-coverage \
	trommel-facts

.DEFAULT_GOAL := help

# Список целей собирается из комментариев вида '## описание' в самом Makefile:
# описание живёт рядом с целью, поэтомуновая цель попадает в вывод без правки в двух местах.
help: ## Список команд с описаниями
	@printf 'Команды окружения SDD Developer Kit:\n\n'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sed 's/:.*## /|/' \
		| awk -F'|' '{printf "  make %-16s %s\n", $$1, $$2}'
	@printf '\nПорядок установки и переменные окружения — в README.md проекта kit'"'"'а.\n'

# Значение переменной из .env: файл заполняется пользователем и на момент первого
# запуска init может быть неполным, поэтому пустое значение заменяется умолчанием
# из .env.example. Разбор построчный, а не через include: .env — файл секретов,
# и его содержимое не должно попадать в пространство имён переменных make.
env_value = $$(sed -n 's/^$(1)=//p' .env 2>/dev/null | tail -n 1)

# Разбор SSH-переменных из .env с умолчаниями. Собран в одном месте и переиспользуется
# шагами подготовки: значения не расходятся между шагами, а добавление переменной не
# требует править каждый шаг. Каждый шаг подставляет присвоения в начало своей строки
# рецепта — переменные shell не переживают переход к следующей строке.
ssh_vars = ssh_key="$(call env_value,SSH_KEY)"; ssh_key="$${ssh_key:-.ssh/id_ed25519}"; \
	ssh_config="$(call env_value,SSH_CONFIG)"; ssh_config="$${ssh_config:-.ssh/config}"; \
	git_host="$(call env_value,GIT_HOST)"; \
	git_user="$(call env_value,GIT_USER)"; git_user="$${git_user:-git}"; \
	container_ssh="$(call env_value,CONTAINER_SSH_DIR)"; container_ssh="$${container_ssh:-/home/claude/.ssh}"

# Единая точка входа подготовки: хостовая часть и инструменты SDD. Под-цели вызываются
# рецептом, а не перечислены зависимостями: зависимости при make -j пошли бы параллельно,
# а обе под-цели пишут в общие файлы корня проекта.
init: ## Полная подготовка проекта: хостовая часть и инструменты SDD
	@$(MAKE) --no-print-directory init-host
	@$(MAKE) --no-print-directory openspec-init
	@echo "Готово. Дальше: заполнить .env (GIT_HOST, CLAUDE_PROFILE) и создать каталог профиля в .claude-accounts/"

# Хостовая часть подготовки — всё, что делается до контейнера: файл секретов, каталоги ключей
# и профилей, записи в .gitignore, SSH-ключ проекта и конфигурация SSH для git-хоста. Шаги
# вызываются рецептом, а не перечислены зависимостями: зависимости при make -j пошли бы
# параллельно, а шаги пишут в общие файлы корня проекта.
#
# Цель остаётся отдельной: хостовую подготовку повторяют после заполнения .env, не трогая
# развёртывание инструментов SDD.
init-host: ## Хостовая подготовка: .env, каталоги, SSH-ключ и конфигурация SSH, .gitignore
	@$(MAKE) --no-print-directory init-env
	@$(MAKE) --no-print-directory init-dirs
	@$(MAKE) --no-print-directory init-gitignore
	@$(MAKE) --no-print-directory init-ssh-key
	@$(MAKE) --no-print-directory init-ssh-config

# Файл секретов. Существующий .env не перезаписывается: в нём заполненные пользователем значения,
# а .env.example — только умолчания.
init-env: ## Создать .env из .env.example
	@if [ -f .env ]; then \
		echo "  .env уже существует — оставлен без изменений"; \
	else \
		cp .env.example .env; \
		echo "  создан .env из .env.example"; \
	fi

# Каталоги ключей и профилей аккаунтов. Права 700 на .ssh — требование ssh-клиента: с более
# широкими правами он отказывается работать с лежащим внутри ключом.
init-dirs: ## Создать каталоги .ssh и .claude-accounts
	@mkdir -p .ssh .claude-accounts
	@chmod 700 .ssh

# Записи в .gitignore. Каждая добавляется однократно: повторный прогон находит её точным
# совпадением строки и пропускает. Перевод строки дописывается перед записью, если файл им
# не заканчивается, — иначе запись склеилась бы с последней строкой.
init-gitignore: ## Добавить в .gitignore .env, .ssh/ и .claude-accounts/
	@touch .gitignore
	@for entry in .env .ssh/ .claude-accounts/; do \
		grep -qxF "$$entry" .gitignore >/dev/null 2>&1 && continue; \
		[ -s .gitignore ] && [ -n "$$(tail -c 1 .gitignore)" ] && printf '\n' >> .gitignore; \
		printf '%s\n' "$$entry" >> .gitignore; \
		echo "  в .gitignore добавлено: $$entry"; \
	done

# SSH-ключ проекта. Существующий ключ не перезаписывается: он может быть уже зарегистрирован
# в git-сервисе. Проверка ssh-keygen — часть этого шага: без утилиты бессмысленна именно
# генерация ключа, и при отдельном вызове цели проверка обязана выполниться.
init-ssh-key: ## Создать SSH-ключ проекта (ed25519)
	@if ! command -v ssh-keygen >/dev/null 2>&1; then \
		echo "Ошибка: не найдена утилита ssh-keygen — установите пакет openssh-client" >&2; \
		exit 1; \
	fi
	@$(ssh_vars); \
	mkdir -p "$$(dirname "$$ssh_key")"; \
	if [ -f "$$ssh_key" ]; then \
		echo "  SSH-ключ $$ssh_key уже существует — оставлен без изменений"; \
	else \
		ssh-keygen -q -t ed25519 -f "$$ssh_key" -N "" -C "sdd-developer-kit@$$(basename "$$(pwd)")"; \
		chmod 600 "$$ssh_key"; \
		chmod 644 "$$ssh_key.pub"; \
		echo "  создан SSH-ключ $$ssh_key (ed25519, без passphrase)"; \
		echo ""; \
		echo "  Добавьте публичный ключ в git-сервис — без этого push из контейнера не пройдёт:"; \
		echo ""; \
		cat "$$ssh_key.pub"; \
		echo ""; \
	fi

# Конфигурация SSH для git-хоста. Существующая конфигурация не перезаписывается: в ней могут быть
# правки пользователя. Путь ключа записывается от каталога SSH внутри контейнера — файл читает
# ssh из контейнера, а не с хоста.
init-ssh-config: ## Создать конфигурацию SSH для git-хоста из .env
	@$(ssh_vars); \
	mkdir -p "$$(dirname "$$ssh_config")"; \
	if [ -f "$$ssh_config" ]; then \
		echo "  $$ssh_config уже существует — оставлен без изменений"; \
	elif [ -z "$$git_host" ]; then \
		echo "  GIT_HOST не задан — $$ssh_config не создан;"; \
		echo "  заполните GIT_HOST в .env и выполните make init повторно"; \
	else \
		printf 'Host %s\n  HostName %s\n  User %s\n  IdentityFile %s/%s\n  IdentitiesOnly yes\n' \
			"$$git_host" "$$git_host" "$$git_user" "$$container_ssh" "$$(basename "$$ssh_key")" \
			> "$$ssh_config"; \
		chmod 644 "$$ssh_config"; \
		echo "  создан $$ssh_config: хост $$git_host, ключ $$container_ssh/$$(basename "$$ssh_key")"; \
	fi

# Инициализация OpenSpec в проекте. Skills, команды агента и каталог openspec/ создаёт сама
# утилита из образа, а не поставка kit'а: иначе они остаются от той версии, что лежала
# в архиве, и расходятся с OPENSPEC_VERSION образа. Язык артефактов задаётся ключом --language,
# поэтому openspec/config.yaml тоже не входит в поставку.
#
# Цель вызывается из init и остаётся отдельной: после смены версии OpenSpec в образе
# инструменты обновляются повторным вызовом, без прохода по хостовой части подготовки.
openspec-init: ## Развернуть инструменты SDD: openspec init в контейнере агента
	@if [ ! -f .env ]; then \
		echo "Ошибка: нет файла .env — сначала выполните make init" >&2; \
		exit 1; \
	fi
	@profile="$${CLAUDE_PROFILE:-$(call env_value,CLAUDE_PROFILE)}"; \
	profile="$${profile:-__no_profile__}"; \
	accounts="$${CLAUDE_ACCOUNTS_DIR:-$(call env_value,CLAUDE_ACCOUNTS_DIR)}"; \
	accounts="$${accounts:-.claude-accounts}"; \
	mkdir -p "$$accounts/$$profile"
	docker compose --profile claude run --rm -T claude \
		openspec init --tools claude --language ru

# Тулчейн Go живёт в контейнере сборки: на хост ничего не ставится, версия фиксируется тегом
# образа, и сборка воспроизводима на любой машине с docker. Цели вызываются на хосте — в
# контейнере агента docker недоступен намеренно.
#
# Кеши уводятся в /tmp контейнера: процесс исполняется от пользователя хоста, и домашнего
# каталога у него внутри образа нет.
GO_IMAGE ?= golang:1.27.1
GO_RUN = docker run --rm -v "$(CURDIR)":/src -w /src -u "$$(id -u):$$(id -g)" \
	-e GOCACHE=/tmp/.gocache -e GOMODCACHE=/tmp/.gomodcache $(GO_IMAGE)

trommel-tools: ## Показать версии инструментов сборки Trommel на хосте
	@printf 'go:     '; go version 2>/dev/null || echo 'не установлен'
	@printf 'docker: '; docker --version 2>/dev/null || echo 'не установлен'
	@printf 'make:   '; $(MAKE) --version 2>/dev/null | head -1

trommel-fmt: ## Отформатировать исходники Trommel
	@$(GO_RUN) gofmt -w .

trommel-deps: ## Привести зависимости модуля в порядок (go mod tidy)
	@$(GO_RUN) go mod tidy

trommel-toolchain: ## Проверить тулчейн Go в контейнере сборки
	@$(GO_RUN) go version

trommel-build: ## Собрать Trommel в контейнере сборки
	@$(GO_RUN) go build -o bin/trommel ./cmd/trommel

trommel-test: ## Прогнать тесты и статические проверки Trommel в контейнере сборки
	@$(GO_RUN) sh -c 'test -z "$$(gofmt -l .)" || { echo "не отформатировано:"; gofmt -l .; exit 1; }'
	@$(GO_RUN) sh -c 'go vet ./... && go test ./...'

# Образ Trommel. Тег по умолчанию — рабочий: выпускной тег задаёт пайплайн, а не рабочее
# дерево, иначе версия образа зависела бы от того, кто его собрал.
TROMMEL_IMAGE ?= trommel:dev

# Внешний анализатор кода в образе: версия выпуска, имя актива и его контрольная сумма.
# Значения живут здесь, а не в .env: их потребляет сборка образа на хосте, а не сервисы
# compose. Сумма закрепляет содержимое актива — подмена валит сборку образа.
TLDR_VERSION ?= v0.4.0
TLDR_ASSET ?= tldr-cli-x86_64-unknown-linux-gnu.tar.xz
TLDR_SHA256 ?= 1455914111af163270dce630dd6c0293805c4a482c90689bd74bd9db49fb80bb

trommel-image: ## Собрать образ Trommel
	@docker build -t $(TROMMEL_IMAGE) \
		--build-arg GO_IMAGE=$(GO_IMAGE) \
		--build-arg TLDR_VERSION=$(TLDR_VERSION) \
		--build-arg TLDR_ASSET=$(TLDR_ASSET) \
		--build-arg TLDR_SHA256=$(TLDR_SHA256) .

# Проверка свойств образа сразу: под кем исполняется процесс [DOCK-012], кому принадлежит
# рабочий каталог [DOCK-013], запускается ли анализатор закреплённой версии и остаётся ли
# смонтированный проект недоступным для записи. Успех последней проверки — именно НЕудача
# записи: запись в проверяемый проект запрещена.
trommel-image-verify: ## Проверить образ: пользователь, владелец каталога, анализатор, монтирование
	@printf 'пользователь:  '; docker run --rm --entrypoint id $(TROMMEL_IMAGE) -un
	@printf 'каталог:       '; docker run --rm --entrypoint stat $(TROMMEL_IMAGE) -c '%U:%G %n' /srv/trommel
	@printf 'анализатор:    '; docker run --rm --entrypoint tldr $(TROMMEL_IMAGE) --version
	@printf 'разбор:        '; docker run --rm -v "$(CURDIR)":/project:ro --entrypoint sh \
		$(TROMMEL_IMAGE) -c 'tldr structure /project --format json --quiet \
		| grep -o "\"path\":" | wc -l' | sed 's/^ *//;s/$$/ файлов разобрано/'
	@if docker run --rm -v "$(CURDIR)":/project:ro --entrypoint sh $(TROMMEL_IMAGE) \
		-c 'touch /project/.trommel-probe' 2>/dev/null; then \
		echo 'монтирование:  ОШИБКА — запись в проверяемый проект удалась'; \
		rm -f .trommel-probe; \
		exit 1; \
	else \
		echo 'монтирование:  только чтение, запись отклонена'; \
	fi

# Сводка покрытия — порождаемый документ: он отвечает, чего стоит зелёный прогон.
# Правится не руками, а этой целью; расхождение файла с выводом прогона означает,
# что цель не вызвали после изменения нормы или реализаций.
#
# projects/ выведен из области: туда монтируются чужие проекты для съёма фактов,
# и их документы к норме Trommel отношения не имеют.
trommel-coverage: ## Обновить сводку покрытия COVERAGE.md
	@./bin/trommel --contract full --exclude-dir=openspec --exclude-dir=projects \
		--format markdown > COVERAGE.md
	@echo 'обновлено: COVERAGE.md'

# Съём фактов о коде с проекта. Путь к проекту задаётся НА ХОСТЕ: каталог projects/ внутри
# контейнера агента — это монтирования compose, на хосте он пуст, и съём по нему дал бы
# пустой срез вместо отказа. Пустой каталог поэтому проверяется явно.
#
# Съём выполняется образом Trommel: анализатор живёт в нём, на хост не ставится. Проект
# монтируется только на чтение — съём не имеет права править снимаемое.
FACTS_DIR ?= facts
FACTS_FORMAT ?= text
facts_name = $(notdir $(abspath $(PROJECT)))
facts_file = $(FACTS_DIR)/$(facts_name).$(if $(filter json,$(FACTS_FORMAT)),json,txt)

trommel-facts: ## Снять срез фактов: PROJECT=<путь на хосте> [SCAN=lib] [LEVELS=L1,L2] [FACTS_FORMAT=json]
	@test -n "$(PROJECT)" || { echo "Ошибка: задайте PROJECT=<путь к проекту на хосте>" >&2; exit 1; }
	@test -d "$(PROJECT)" || { echo "Ошибка: каталога $(PROJECT) на хосте нет" >&2; exit 1; }
	@test -n "$$(ls -A "$(PROJECT)")" || { \
		echo "Ошибка: $(PROJECT) пуст. Путь задаётся на хосте: внутри контейнера агента" >&2; \
		echo "каталог projects/ — это монтирование compose, на хосте он пустой." >&2; exit 1; }
	@mkdir -p $(FACTS_DIR)
	@docker run --rm -v "$(abspath $(PROJECT))":/project:ro $(TROMMEL_IMAGE) \
		--facts --project /project --format $(FACTS_FORMAT) \
		$(foreach dir,$(SCAN),--scan $(dir)) $(foreach level,$(LEVELS),--level $(level)) \
		> $(facts_file)
	@echo "срез: $(facts_file) ($$(wc -c < $(facts_file)) байт)"
	@if [ "$(FACTS_FORMAT)" != json ]; then cat $(facts_file); fi
