# port-selector

[![CI](https://github.com/dapi/port-selector/actions/workflows/ci.yml/badge.svg)](https://github.com/dapi/port-selector/actions/workflows/ci.yml)
[![Release](https://github.com/dapi/port-selector/actions/workflows/release.yml/badge.svg)](https://github.com/dapi/port-selector/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/dapi/port-selector)](https://goreportcard.com/report/github.com/dapi/port-selector)
[![Parallel AI Agents](https://img.shields.io/badge/Parallel_AI-Agents_Ready-00d4aa)](https://github.com/dapi/port-selector)

[🇬🇧 English version](README.md)

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

## Дополнительные материалы

Практика запуска нескольких AI-агентов параллельно с использованием git worktrees становится всё более популярной. Каждый worktree обеспечивает полную изоляцию файлов, но все агенты по-прежнему используют общие сетевые ресурсы — включая порты. Когда агенты запускают dev-серверы, e2e-тесты или preview-деплойменты, конфликты портов неизбежны.

`port-selector` решает эту проблему, обеспечивая автоматическое выделение портов с периодом заморозки — каждый агент получает уникальный порт, даже если несколько агентов стартуют одновременно.

**Статьи о параллельной разработке с AI-агентами:**

- [How we're shipping faster with Claude Code and Git Worktrees](https://incident.io/blog/shipping-faster-with-claude-code-and-git-worktrees) — опыт incident.io с несколькими сессиями Claude Code и кастомным менеджером worktree
- [Parallel AI Development with Git Worktrees](https://sgryt.com/posts/git-worktree-parallel-ai-development/) — «три столпа»: изоляция состояния, параллельное выполнение, асинхронная интеграция
- [How Git Worktrees Changed My AI Agent Workflow](https://nx.dev/blog/git-worktrees-ai-agents) — практические сценарии, когда агенты работают в фоне, пока вы продолжаете кодить
- [Git Worktrees: The Secret Weapon for Running Multiple AI Agents](https://medium.com/@mabd.dev/git-worktrees-the-secret-weapon-for-running-multiple-ai-coding-agents-in-parallel-e9046451eb96) — почему worktrees стали необходимы в эпоху AI-разработки
- [Parallel Coding Agents with Container Use and Git Worktree](https://www.youtube.com/watch?v=z1osqcNQRvw) — видео-обзор трёх workflow для параллельных агентов

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
make install
```

Это соберёт бинарник и установит его в `/usr/local/bin/`.

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

### Привязка порта к директории

Каждая директория автоматически получает свой выделенный порт. Запуск `port-selector` из одной директории всегда возвращает один и тот же порт:

```bash
$ cd ~/projects/project-a
$ port-selector
3000

$ cd ~/projects/project-b
$ port-selector
3001

$ cd ~/projects/project-a
$ port-selector
3000  # Тот же порт!
```

Особенно полезно с git worktree — каждый worktree получает стабильный порт.

### Управление аллокациями

```bash
# Показать все аллокации портов
port-selector --list

# Вывод:
# PORT  STATUS  DIRECTORY                    ASSIGNED
# 3000  free    /home/user/projects/app-a    2025-01-02 10:30
# 3001  busy    /home/user/projects/app-b    2025-01-02 11:45

# Удалить аллокацию для текущей директории
cd ~/projects/old-project
port-selector --forget
# Cleared allocation for /home/user/projects/old-project (was port 3005)

# Удалить все аллокации
port-selector --forget-all
# Cleared 5 allocation(s)
```

### Аргументы командной строки

```
port-selector [options]

Options:
  -h, --help      Показать справку
  -v, --version   Показать версию
  -l, --list      Показать все аллокации портов
  --forget        Удалить аллокацию для текущей директории
  --forget-all    Удалить все аллокации
```

## Конфигурация

При первом запуске создаётся файл конфигурации:

**~/.config/port-selector/default.yaml**

```yaml
# Начальный порт диапазона
portStart: 3000

# Конечный порт диапазона
portEnd: 4000

# Период заморозки порта после выдачи (в минутах)
# Порт не будет переиспользован в течение этого времени
# 0 = отключено, 1440 = 24 часа (по умолчанию)
freezePeriodMinutes: 1440

# Автоматическое удаление аллокаций после указанного периода
# Поддерживает: 30d (дни), 720h (часы), 24h30m (комбинированный формат)
# Пустая строка или "0" = отключено (по умолчанию)
allocationTTL: 30d
```

### TTL аллокаций

Когда `allocationTTL` установлен, аллокации старше указанного периода автоматически удаляются при каждом запуске. Это предотвращает накопление устаревших аллокаций от удалённых проектов:

```yaml
allocationTTL: 30d  # Аллокации истекают после 30 дней неактивности
```

Временная метка обновляется каждый раз, когда порт возвращается для существующей аллокации, поэтому активно используемые аллокации никогда не истекают.

### Период заморозки (Freeze Period)

После выдачи порта он "замораживается" на указанное время и не будет выдан повторно. Это решает проблему, когда приложение медленно стартует и порт кажется свободным, хотя на нём вот-вот запустится другой сервер.

```
Время 10:00 - Agent 1 запросил порт → получил 3000
Время 10:01 - Agent 2 запросил порт → получил 3001 (3000 заморожен)
Время 10:02 - Agent 1 остановился, порт 3000 освободился
Время 10:03 - Agent 3 запросил порт → получил 3002 (3000 всё ещё заморожен)
...
Время 34:01 - Прошло 24 часа, порт 3000 разморожен
```

История выданных портов хранится в `~/.config/port-selector/issued-ports.yaml` и автоматически очищается от устаревших записей.

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
│  2. Читаем last-used и историю         │
│     last-used → начальная точка        │
│     issued-ports.yaml → замороженные   │
└──────────────────┬─────────────────────┘
                   │
                   ▼
┌────────────────────────────────────────┐
│  3. Проверяем порт:                    │
│     - Не заморожен?                    │
│     - Свободен? (net.Listen)           │
└──────────────────┬─────────────────────┘
                   │
           ┌───────┴───────┐
           │               │
      подходит     заморожен/занят
           │               │
           ▼               ▼
┌──────────────────┐ ┌──────────────────┐
│ 4a. Сохраняем:   │ │ 4b. Следующий    │
│  - last-used     │ │     порт         │
│  - в историю     │ │     (wrap-around │
│  Выводим STDOUT  │ │     после конца) │
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
make test

# Собрать
make build

# Собрать и установить в /usr/local/bin
make install

# Удалить
make uninstall
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
│   ├── history/
│   │   └── history.go       # История выданных портов (freeze period)
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

[Danil Pismenny](https://pismenny.ru) ([@dapi](https://github.com/dapi))
