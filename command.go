package command

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "os"
    "strings"
    "time"

    "gopkg.in/yaml.v3"
)

// ResponseType определяет тип ответа
type ResponseType string

const (
    TypeFinal   ResponseType = "final"
    TypeCommand ResponseType = "command"
)

// CommandContext контекст предыдущей команды createLatex
type CommandContext struct {
    Script string `json:"script"` // существующий LaTeX скрипт
    Text   string `json:"text"`    // исходное математическое выражение
}

// CommandRequest запрос в модуль command-qwen
type CommandRequest struct {
    Context     *CommandContext `json:"context"`      // предыдущая команда createLatex (если была)
    CurrentText string          `json:"current_text"` // текущая фраза (уже проверена isMath)
}

// CommandResponse ответ от модуля command-qwen
type CommandResponse struct {
    Type   ResponseType `json:"type"`            // "command"
    Name   string       `json:"name"`            // "createLatex" или "editLatex"
    Script string       `json:"script"`          // LaTeX скрипт
    Text   string       `json:"text"`            // исходный текст (для create - контекст, для edit - текущий)
}

// QwenConfig конфигурация для Ollama
type QwenConfig struct {
    Model      string `yaml:"model"`       // "qwen2.5-0.5b.local"
    URL        string `yaml:"url"`         // "http://localhost:11434/api/chat"
    TimeoutSec int    `yaml:"timeout_sec"` // 5
}

// Config полная конфигурация
type Config struct {
    Qwen QwenConfig `yaml:"qwen"`
}

// CommandResolver главный обработчик
type CommandResolver struct {
    config     *Config
    httpClient *http.Client
    logger     *log.Logger
}

// NewResolver создает новый резолвер
func NewResolver(configPath string) (*CommandResolver, error) {
    data, err := os.ReadFile(configPath)
    if err != nil {
        return nil, fmt.Errorf("read config: %w", err)
    }

    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("parse config: %w", err)
    }

    httpClient := &http.Client{
        Timeout: time.Duration(cfg.Qwen.TimeoutSec) * time.Second,
    }

    return &CommandResolver{
        config:     &cfg,
        httpClient: httpClient,
        logger:     log.New(log.Writer(), "[QWEN] ", log.LstdFlags),
    }, nil
}

// determineCommandType определяет, создание это или модификация
func (r *CommandResolver) determineCommandType(req CommandRequest) (string, string, string) {
    if req.Context == nil {
        // Нет контекста → создание новой формулы
        return "CREATE", req.CurrentText, ""
    }

    // Есть контекст → модификация существующей
    return "EDIT", req.Context.Script, req.CurrentText
}

// buildPrompt создает промпт для Qwen
// - Output ONLY the LaTeX code, no dollar signs ($), no explanations

func (r *CommandResolver) buildPrompt(cmdType, param1, param2 string) string {
    switch cmdType {
    case "CREATE":
        return fmt.Sprintf(`Convert this Russian mathematical phrase to LaTeX code.

RULES:
- Output ONLY the LaTeX code, no explanations
- Keep it simple and clean
- Use standard LaTeX commands

EXAMPLES:
Russian: "два плюс три"
LaTeX: $2+3$

Russian: "икс"
LaTeX: $x$

Russian: "интеграл от икс"
LaTeX: $\int x$

Russian: "корень из двух"
LaTeX: $\sqrt{2}$

Russian: "дробь один плюс икс на два"
LaTeX: $\frac{1+x}{2}$

Russian: "сумма от икс равно один до пяти"
LaTeX: $\sum_{x=1}^{5}$

NOW CONVERT THIS:
Russian: "%s"
LaTeX:`, param1)

    case "EDIT":
        return fmt.Sprintf(`Modify the existing LaTeX code according to the instruction.

RULES:
- Output ONLY the modified LaTeX code, no explanations
- Keep the original structure and add/modify as instructed
- Use standard LaTeX syntax

EXAMPLES:
Current LaTeX: $2+3$
Instruction: "добавь корень из трех"
Modified: $2+3+\sqrt{3}$

Current LaTeX: $\int x$
Instruction: "измени икс на икс в квадрате"
Modified: $\int x^2$

Current LaTeX: $\sqrt{2}$
Instruction: "добавь плюс один"
Modified: $\sqrt{2}+1$

NOW MODIFY THIS:
Current LaTeX: %s
Instruction: "%s"
Modified:`, param1, param2)

    default:
        return ""
    }
}

// Resolve обрабатывает запрос
func (r *CommandResolver) Resolve(ctx context.Context, req CommandRequest) (*CommandResponse, error) {
    // 1. Определяем тип команды
    cmdType, param1, param2 := r.determineCommandType(req)

    r.logger.Printf("Command type: %s, input: %q | %q", cmdType, param1, param2)

    // 2. Формируем промпт
    prompt := r.buildPrompt(cmdType, param1, param2)

    // 3. Отправляем в Qwen
    payload := map[string]interface{}{
        "model": r.config.Qwen.Model,
        "messages": []map[string]string{
            {"role": "user", "content": prompt},
        },
        "stream": false,
        "options": map[string]interface{}{
            "temperature": 0.0,
            "max_tokens":  200,
        },
    }

    jsonData, err := json.Marshal(payload)
    if err != nil {
        return nil, fmt.Errorf("marshal payload: %w", err)
    }

    // r.logger.Printf("Sending to Qwen: %s", prompt)

    httpReq, err := http.NewRequestWithContext(ctx, "POST", r.config.Qwen.URL, strings.NewReader(string(jsonData)))
    if err != nil {
        return nil, fmt.Errorf("create request: %w", err)
    }
    httpReq.Header.Set("Content-Type", "application/json")

    resp, err := r.httpClient.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("http request: %w", err)
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("read response: %w", err)
    }

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("ollama status %d: %s", resp.StatusCode, string(body))
    }

    // 4. Парсим ответ Ollama
    var ollamaResp struct {
        Message struct {
            Content string `json:"content"`
        } `json:"message"`
    }

    if err := json.Unmarshal(body, &ollamaResp); err != nil {
        return nil, fmt.Errorf("parse ollama response: %w", err)
    }

    r.logger.Printf("Qwen response: %s", ollamaResp.Message.Content)

    // 5. Формируем ответ
    script := strings.TrimSpace(ollamaResp.Message.Content)

    if cmdType == "CREATE" {
        return &CommandResponse{
            Type:   TypeCommand,
            Name:   "createLatex",
            Script: script,
            Text:   req.CurrentText, // исходное математическое выражение
        }, nil
    } else {
        return &CommandResponse{
            Type:   TypeCommand,
            Name:   "editLatex",
            Script: script,
            Text:   req.CurrentText, // инструкция по модификации
        }, nil
    }
}

// Close освобождает ресурсы
func (r *CommandResolver) Close() error {
    r.httpClient.CloseIdleConnections()
    return nil
}
