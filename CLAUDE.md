# CLAUDE.md - Инструкции для AI-агентов

## О проекте

**port-selector** — CLI утилита на Go для автоматического выбора свободного порта из заданного диапазона. Предназначена для использования в AI-driven разработке с множеством параллельных агентов.

## Технический стек

- **Язык:** Go 1.21+
- **Версионирование:** mise (.mise.toml)
- **CI/CD:** GitHub Actions
- **Релизы:** goreleaser или ручная сборка через workflow

## Структура проекта

```
port-selector/
├── cmd/port-selector/main.go    # Точка входа, парсинг аргументов
├── internal/
│   ├── config/config.go         # Чтение/создание YAML конфига
│   ├── cache/cache.go           # Работа с last-used файлом
│   └── port/checker.go          # Проверка доступности портов
├── .github/workflows/release.yml
├── .mise.toml
├── go.mod
└── go.sum
```

## Ключевые требования

### Функциональные

1. **Без аргументов** → выводит свободный порт в STDOUT
2. **-h, --help** → справка
3. **-v, --version** → версия (встраивается при сборке через `-ldflags`)
4. **Конфиг** в `~/.config/port-selector/default.yaml`:
   ```yaml
   portStart: 3000
   portEnd: 4000
   ```
5. **Кеш** последнего порта в `~/.config/port-selector/last-used`
6. **Wrap-around** — после достижения portEnd начинаем с portStart
7. **Ошибка** в STDERR с exit code 1, если все порты заняты

### Нефункциональные

- Минимум зависимостей (только stdlib Go + yaml парсер)
- Быстрый запуск (< 100ms)
- Атомарная запись кеша (для предотвращения гонок)

## Команды разработки

```bash
# Запуск тестов
go test ./... -v

# Сборка
go build -o port-selector ./cmd/port-selector

# Сборка с версией
go build -ldflags "-X main.version=1.0.0" -o port-selector ./cmd/port-selector

# Проверка линтером
golangci-lint run

# Форматирование
go fmt ./...
```

## Паттерны кода

### Проверка порта

```go
func IsPortFree(port int) bool {
    ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
    if err != nil {
        return false
    }
    ln.Close()
    return true
}
```

### Работа с конфигом

- Используй `os.UserConfigDir()` для кроссплатформенности
- Создавай директории с `os.MkdirAll(..., 0755)`
- Используй `gopkg.in/yaml.v3` для YAML

### Обработка ошибок

```go
// Вывод ошибок в STDERR
fmt.Fprintln(os.Stderr, "error: all ports in range are busy")
os.Exit(1)

// Успешный вывод в STDOUT (только порт!)
fmt.Println(port)
```

## Тестирование

### Unit-тесты

- Тестируй каждый пакет отдельно
- Используй table-driven tests
- Mock файловую систему через интерфейсы

### Integration-тесты

```go
func TestFindFreePort(t *testing.T) {
    // Занимаем порт
    ln, _ := net.Listen("tcp", ":3000")
    defer ln.Close()

    // Проверяем, что вернётся следующий
    port := FindFreePort(3000, 3010, 3000)
    if port == 3000 {
        t.Error("should skip busy port")
    }
}
```

## GitHub Actions

### Release workflow

При создании тега `v*` должен:
1. Собрать бинарники для linux/darwin (amd64/arm64)
2. Встроить версию из тега
3. Загрузить артефакты в релиз

```yaml
# Пример ldflags
-ldflags "-X main.version=${{ github.ref_name }}"
```

## Важные детали

1. **Только STDOUT для порта** — никакого дополнительного текста
2. **Кеш атомарный** — записывай во временный файл, потом переименовывай
3. **Graceful handling** — если нет прав на создание конфига, продолжай с дефолтами
4. **Не блокируй порт** — только проверяй и сразу закрывай listener

## Пример использования

```bash
# Агент запускает
$ port-selector
3000

# Следующий агент
$ port-selector
3001

# Использование в скрипте
$ npm run dev -- --port $(port-selector)
```

## Чеклист перед коммитом

- [ ] Тесты проходят: `go test ./...`
- [ ] Код отформатирован: `go fmt ./...`
- [ ] Нет ошибок линтера: `golangci-lint run`
- [ ] Бинарник собирается: `go build ./cmd/port-selector`
- [ ] README актуален
