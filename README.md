# port-selector

[![CI](https://github.com/dapi/port-selector/actions/workflows/ci.yml/badge.svg)](https://github.com/dapi/port-selector/actions/workflows/ci.yml)
[![Release](https://github.com/dapi/port-selector/actions/workflows/release.yml/badge.svg)](https://github.com/dapi/port-selector/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/dapi/port-selector)](https://goreportcard.com/report/github.com/dapi/port-selector)

CLI утилита для автоматического выбора свободного порта из заданного диапазона.

## Мотивация

При разработке с использованием AI-агентов (Claude Code, Cursor, Copilot Workspace и др.) часто возникает ситуация, когда множество параллельных агентов работают над задачами в отдельных git worktree. Каждый агент может запускать веб-серверы для e2e-тестирования, и всем им нужны свободные порты.

**Проблема:** Когда 5-10 агентов одновременно пытаются запустить dev-серверы на порту 3000, возникают конфликты.

**Решение:** `port-selector` автоматически находит и выдаёт первый свободный порт из настроенного диапазона.

```
┌─────────────────────────────────────────────────────────────┐
│  Agent 1 (worktree: feature-auth)                           │
│  $ PORT=$(port-selector) && npm run dev -- --port $PORT     │
│  → Server running on http://localhost:3000                  │
├─────────────────────────────────────────────────────────────┤
│  Agent 2 (worktree: feature-dashboard)                      │
│  $ PORT=$(port-selector) && npm run dev -- --port $PORT     │
│  → Server running on http://localhost:3001                  │
├─────────────────────────────────────────────────────────────┤
│  Agent 3 (worktree: bugfix-login)                           │
│  $ PORT=$(port-selector) && npm run dev -- --port $PORT     │
│  → Server running on http://localhost:3002                  │
└─────────────────────────────────────────────────────────────┘
```

## Установка

### Из релизов GitHub

```bash
# Linux (amd64)
curl -L https://github.com/dapi/port-selector/releases/latest/download/port-selector-linux-amd64 -o port-selector
chmod +x port-selector
sudo mv port-selector /usr/local/bin/

# macOS (arm64 - Apple Silicon)
curl -L https://github.com/dapi/port-selector/releases/latest/download/port-selector-darwin-arm64 -o port-selector
chmod +x port-selector
sudo mv port-selector /usr/local/bin/

# macOS (amd64 - Intel)
curl -L https://github.com/dapi/port-selector/releases/latest/download/port-selector-darwin-amd64 -o port-selector
chmod +x port-selector
sudo mv port-selector /usr/local/bin/
```

### Сборка из исходников

```bash
git clone https://github.com/dapi/port-selector.git
cd port-selector
go build -o port-selector ./cmd/port-selector
```

## Использование

### Базовое использование

```bash
# Получить свободный порт
port-selector
# Вывод: 3000

# Использовать в скрипте
PORT=$(port-selector)
npm run dev -- --port $PORT

# Или в одну строку
npm run dev -- --port $(port-selector)
```

### Примеры интеграции

#### Next.js / Vite / любой dev-сервер

```bash
# package.json scripts
{
  "scripts": {
    "dev": "PORT=$(port-selector) next dev -p $PORT",
    "dev:vite": "vite --port $(port-selector)"
  }
}
```

#### Docker Compose

```bash
# В .env или при запуске
export APP_PORT=$(port-selector)
docker-compose up
```

#### Playwright / e2e тесты

```bash
# В конфиге playwright
export BASE_URL="http://localhost:$(port-selector)"
npx playwright test
```

#### direnv (.envrc)

Идеальный способ для проектов с git worktree — порт назначается автоматически при входе в директорию:

```bash
# .envrc
export PORT=$(port-selector)

# Теперь в любом скрипте проекта используйте $PORT
# npm run dev автоматически получит свой уникальный порт
```

```bash
# Пример workflow с git worktree
$ cd ~/projects/myapp-feature-auth
direnv: loading .envrc
direnv: export +PORT

$ echo $PORT
3000

$ cd ~/projects/myapp-feature-dashboard
direnv: loading .envrc
direnv: export +PORT

$ echo $PORT
3001
```

#### Claude Code / AI агенты

Добавьте в CLAUDE.md вашего проекта:

```markdown
## Запуск dev-сервера

Перед запуском dev-сервера всегда используй port-selector:
\`\`\`bash
PORT=$(port-selector) npm run dev -- --port $PORT
\`\`\`
```

### Аргументы командной строки

```
port-selector [options]

Options:
  -h, --help     Показать справку
  -v, --version  Показать версию
```

## Конфигурация

При первом запуске создаётся файл конфигурации:

**~/.config/port-selector/default.yaml**

```yaml
# Начальный порт диапазона
portStart: 3000

# Конечный порт диапазона
portEnd: 4000
```

### Кеширование

Для оптимизации утилита запоминает последний выданный порт в `~/.config/port-selector/last-used`. При следующем вызове проверка начинается с этого порта, а не с начала диапазона.

```
Первый вызов:  проверяет 3000 → свободен → возвращает 3000, сохраняет 3000
Второй вызов:  проверяет 3001 → свободен → возвращает 3001, сохраняет 3001
Третий вызов:  проверяет 3002 → занят → проверяет 3003 → свободен → возвращает 3003
...
После 4000:    проверяет 3000 (wrap-around)
```

## Алгоритм работы

```
┌────────────────────────────────────────┐
│          port-selector                 │
└──────────────────┬─────────────────────┘
                   │
                   ▼
┌────────────────────────────────────────┐
│  1. Читаем конфиг                      │
│     ~/.config/port-selector/default.yaml│
│     (создаём если нет)                 │
└──────────────────┬─────────────────────┘
                   │
                   ▼
┌────────────────────────────────────────┐
│  2. Читаем last-used                   │
│     ~/.config/port-selector/last-used  │
│     (если нет → начинаем с portStart)  │
└──────────────────┬─────────────────────┘
                   │
                   ▼
┌────────────────────────────────────────┐
│  3. Проверяем порт на свободность      │
│     net.Listen("tcp", ":port")         │
└──────────────────┬─────────────────────┘
                   │
           ┌───────┴───────┐
           │               │
      свободен          занят
           │               │
           ▼               ▼
┌──────────────────┐ ┌──────────────────┐
│ 4a. Сохраняем    │ │ 4b. Следующий    │
│     в last-used  │ │     порт         │
│     Выводим в    │ │     (wrap-around │
│     STDOUT       │ │     после конца) │
└──────────────────┘ └────────┬─────────┘
                              │
                    ┌─────────┴─────────┐
                    │                   │
              есть ещё          все проверены
                    │                   │
                    ▼                   ▼
              → шаг 3          ┌────────────────┐
                               │ ОШИБКА в STDERR│
                               │ exit code 1    │
                               └────────────────┘
```

## Разработка

### Требования

- Go 1.21+
- mise (для управления версиями)

### Локальная сборка

```bash
# Установить зависимости через mise
mise install

# Запустить тесты
go test ./...

# Собрать
go build -o port-selector ./cmd/port-selector

# Собрать с версией
go build -ldflags "-X main.version=1.0.0" -o port-selector ./cmd/port-selector
```

### Структура проекта

```
port-selector/
├── cmd/
│   └── port-selector/
│       └── main.go          # Точка входа
├── internal/
│   ├── config/
│   │   └── config.go        # Работа с конфигурацией
│   ├── cache/
│   │   └── cache.go         # Кеширование last-used
│   └── port/
│       └── checker.go       # Проверка портов
├── .github/
│   └── workflows/
│       └── release.yml      # GitHub Actions для релизов
├── .mise.toml               # Конфигурация mise
├── go.mod
├── go.sum
├── CLAUDE.md                # Инструкции для AI-агентов
└── README.md
```

## Лицензия

MIT

## Автор

[@dapi](https://github.com/dapi)
