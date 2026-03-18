# command-qwen-gguf

Модуль для работы с LaTeX командами через Qwen 0.5B / 1.5B (через Ollama)

определение команды в контексте предыдущей фразы и ее выполнение

## Архитектура

Vosk → Levenshtein → isMath → command-qwen-gguf → Browser

- **isMath** — отдельный модуль для проверки математических терминов
- **command-qwen-gguf** — только генерация/модификация LaTeX через Qwen

## Команды

- `createLatex` — создание LaTeX-скрипта из математического выражения
- `editLatex` — модификация существующего LaTeX

## Структура

.
├── command.go # основная логика
├── cmd/
│ └── run.go # тестирование
├── config.yaml # конфигурация
└── tests.json # тестовые примеры


## Тестирование

```bash
go run cmd/run.go -config config.yaml -test tests.json

## Благодарность

Огромное спасибо DeepSeek за помощь в проектировании архитектуры, мозговые штурмы и отладку! 🤖✨
