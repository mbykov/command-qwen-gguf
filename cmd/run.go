package main

import (
    "context"
    "encoding/json"
    "flag"
    "fmt"
    "log"
    "os"
    "strings"
    "time"

    "github.com/michael/bhl-qwen-go"
)

// TestCase тестовый пример из JSON файла
type TestCase struct {
    Name        string                   `json:"name"`
    Context     *command.CommandContext  `json:"context"`      // может быть null
    CurrentText string                   `json:"current_text"` // уже проверено isMath
    Expected    *command.CommandResponse `json:"expected"`
}

// TestResults результаты тестирования
type TestResults struct {
    Total    int            `json:"total"`
    Passed   int            `json:"passed"`
    Failed   int            `json:"failed"`
    Failures []TestFailure  `json:"failures,omitempty"`
}

type TestFailure struct {
    Name     string `json:"name"`
    Expected string `json:"expected"`
    Got      string `json:"got"`
}

func main() {
    // Парсим флаги
    configPath := flag.String("config", "config.yaml", "path to config file")
    testFile := flag.String("test", "", "path to test cases JSON file")
    debug := flag.Bool("debug", false, "enable debug output")
    flag.Parse()

    if *testFile == "" {
        log.Fatal("Please provide test file with -test")
    }

    log.SetPrefix("[TEST] ")
    log.Printf("🚀 Запуск тестов модуля command-qwen (debug=%v)", *debug)

    // Загружаем резолвер
    resolver, err := command.NewResolver(*configPath)
    if err != nil {
        log.Fatalf("❌ Failed to create resolver: %v", err)
    }
    defer resolver.Close()

    // Загружаем тесты
    tests, err := loadTests(*testFile)
    if err != nil {
        log.Fatalf("❌ Failed to load tests: %v", err)
    }

    log.Printf("📋 Загружено %d тестов", len(tests))

    // Запускаем тесты
    results := runTests(resolver, tests, *debug)

    // Выводим результаты
    printResults(results)

    // Выходим с ошибкой если есть упавшие тесты
    if results.Failed > 0 {
        os.Exit(1)
    }
}

// loadTests загружает тесты из JSON файла
func loadTests(path string) ([]TestCase, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("read file: %w", err)
    }

    var tests []TestCase
    if err := json.Unmarshal(data, &tests); err != nil {
        return nil, fmt.Errorf("parse JSON: %w", err)
    }

    return tests, nil
}

// runTests запускает все тесты
func runTests(resolver *command.CommandResolver, tests []TestCase, debug bool) TestResults {
    results := TestResults{
        Total:    len(tests),
        Failures: []TestFailure{},
    }

    ctx := context.Background()

    for i, test := range tests {
        log.Printf("🔍 \n Тест %d/%d: %s", i+1, results.Total, test.Name)

        if debug {
            if test.Context != nil {
                log.Printf("   Context: script=%q text=%q", test.Context.Script, test.Context.Text)
            } else {
                log.Printf("   Context: nil")
            }
            log.Printf("   Current: %q", test.CurrentText)
            log.Printf("   Expected: %+v", test.Expected)
        }

        // Создаем запрос
        req := command.CommandRequest{
            Context:     test.Context,
            CurrentText: test.CurrentText,
        }

        // Отправляем в Qwen
        start := time.Now()
        resp, err := resolver.Resolve(ctx, req)
        duration := time.Since(start)

        if err != nil {
            log.Printf("❌ Ошибка: %v", err)
            results.Failed++
            results.Failures = append(results.Failures, TestFailure{
                Name:     test.Name,
                Expected: fmt.Sprintf("%+v", test.Expected),
                Got:      fmt.Sprintf("error: %v", err),
            })
            continue
        }

        if debug {
            log.Printf("   Response: %+v", resp)
        }

        // Проверяем результат
        if !compareResponses(resp, test.Expected) {
            log.Printf("❌ Не совпадает (за %v)", duration.Round(time.Millisecond))
            results.Failed++
            results.Failures = append(results.Failures, TestFailure{
                Name:     test.Name,
                Expected: fmt.Sprintf("%+v", test.Expected),
                Got:      fmt.Sprintf("%+v", resp),
            })
        } else {
            log.Printf("✅ Успешно (за %v)", duration.Round(time.Millisecond))
            results.Passed++
        }

        // Небольшая пауза между запросами
        time.Sleep(500 * time.Millisecond)
    }

    return results
}

// compareResponses сравнивает фактический ответ с ожидаемым
func compareResponses(got, expected *command.CommandResponse) bool {

    if killSpaces(got.Type) != killSpaces(expected.Type) {
        log.Printf(" ❌ Type %v _exp: %v", got.Type, expected.Type )
        return false
    }
    if killSpaces(got.Name) != killSpaces(expected.Name) {
        log.Printf(" ❌ Name %v _exp: %v", got.Name, expected.Name )
        return false
    }
    if killSpaces(got.Script) != killSpaces(expected.Script) {
        log.Printf(" ❌ Script %v _exp: %v", got.Script, expected.Script )
        return false
    }
    if killSpaces(got.Text) != killSpaces(expected.Text) {
        log.Printf(" ❌ Text %v _exp: %v", got.Text, expected.Text )
        return false
    }
    return true
}

func killSpaces(v interface{}) string {
    s := fmt.Sprint(v)
    return strings.ReplaceAll(s, " ", "")
}

// printResults выводит результаты тестирования
func printResults(results TestResults) {
    fmt.Println("\n" + strings.Repeat("=", 50))
    fmt.Println("📊 РЕЗУЛЬТАТЫ ТЕСТИРОВАНИЯ")
    fmt.Println(strings.Repeat("=", 50))
    fmt.Printf("Всего: %d\n", results.Total)
    fmt.Printf("✅ Успешно: %d\n", results.Passed)
    fmt.Printf("❌ Провалено: %d\n", results.Failed)

    if len(results.Failures) > 0 {
        fmt.Println("\n" + strings.Repeat("-", 50))
        fmt.Println("Детали ошибок:")
        for _, f := range results.Failures {
            fmt.Printf("\n🔴 %s\n", f.Name)
            fmt.Printf("   Ожидалось: %s\n", f.Expected)
            fmt.Printf("   Получено:  %s\n", f.Got)
        }
    }

    fmt.Println(strings.Repeat("=", 50))
}
