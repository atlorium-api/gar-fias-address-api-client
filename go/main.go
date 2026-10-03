// Клиент API ГАР/ФИАС Atlorium — поиск и нормализация российских адресов.
//
// Запуск (работает сразу, без регистрации — на демо-ключе):
//
//	go run . "москва тверская"
//
// Второй аргумент — номер выбранной подсказки (по умолчанию 1):
//
//	go run . "москва тверская" 2
//
// Боевой ключ: получить на https://atlorium.com и положить в переменную окружения
// ATLORIUM_API_KEY. Код при этом не меняется.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

// SandboxKey — публичный демо-ключ. С ним API отвечает правдоподобными МОКАМИ
// (не реальными данными ГАР), чтобы можно было встроить интеграцию до оплаты.
// Ответы детерминированы — на них можно писать стабильные тесты.
const SandboxKey = "ak_sandbox_demo_mockdata_v1"

var (
	apiKey  = envOr("ATLORIUM_API_KEY", SandboxKey)
	baseURL = envOr("ATLORIUM_BASE_URL", "https://atlorium.com")
	client  = &http.Client{Timeout: 30 * time.Second}
)

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// ── Модель ответа ────────────────────────────────────────────────────────────

// SuggestItem — подсказка автодополнения. Элементы /suggest лежат в поле items.
type SuggestItem struct {
	Text     string `json:"text"`
	ObjectID int64  `json:"objectId"`
	// ObjectGUID — GUID ФИАС, стабильный идентификатор адреса. Именно он хранится в БД.
	ObjectGUID string `json:"objectGuid"`
	ObjectType string `json:"objectType"`
}

// SuggestResponse — ответ /api/Gar/suggest.
type SuggestResponse struct {
	Query     string        `json:"query"`
	Items     []SuggestItem `json:"items"`
	Count     int           `json:"count"`
	ElapsedMs int           `json:"elapsedMs"`
}

// SearchResult — результат полного поиска. Элементы /search лежат в поле results.
type SearchResult struct {
	FullAddress string `json:"fullAddress"`
	ObjectType  string `json:"objectType"`
	ObjectGUID  string `json:"objectGuid"`
	ObjectID    int64  `json:"objectId"`
	RegionCode  string `json:"regionCode"`
	PostalCode  string `json:"postalCode"`
	Oktmo       string `json:"oktmo"`
	Okato       string `json:"okato"`
}

// SearchResponse — ответ /api/Gar/search.
type SearchResponse struct {
	Query     string         `json:"query"`
	Results   []SearchResult `json:"results"`
	Count     int            `json:"count"`
	ElapsedMs int            `json:"elapsedMs"`
}

// HierarchyLevel — уровень иерархии: регион, город, улица, дом.
type HierarchyLevel struct {
	ObjectID    int64  `json:"objectId"`
	DisplayName string `json:"displayName"`
}

// AddressParameter — пара «тип — значение»: почтовый индекс, ОКТМО, ОКАТО,
// код ИФНС, код КЛАДР.
type AddressParameter struct {
	TypeID   int    `json:"typeId"`
	TypeName string `json:"typeName"`
	Value    string `json:"value"`
}

// AddressObject — карточка адресного объекта ГАР/ФИАС.
type AddressObject struct {
	ObjectID    int64              `json:"objectId"`
	ObjectGUID  string             `json:"objectGuid"`
	FullAddress string             `json:"fullAddress"`
	ObjectType  string             `json:"objectType"`
	RegionCode  string             `json:"regionCode"`
	Hierarchy   []HierarchyLevel   `json:"hierarchy"`
	Parameters  []AddressParameter `json:"parameters"`
	ElapsedMs   int                `json:"elapsedMs"`
}

// APIError раскладывает HTTP-код в человекочитаемую причину.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	reasons := map[int]string{
		400: "неверный запрос (пустая строка поиска или некорректный GUID/objectId)",
		401: "API-ключ отсутствует, просрочен или недействителен",
		402: "недостаточно кредитов на балансе — пополните на https://atlorium.com",
		404: "адресный объект не найден в ГАР/ФИАС",
		429: "превышен лимит запросов — повторите позже",
		500: "внутренняя ошибка при обращении к адресной базе (за сбой на своей стороне мы не списываем деньги)",
		503: "сервис временно недоступен (плановые работы) — повторите позже",
	}
	reason, ok := reasons[e.Status]
	if !ok {
		reason = "неизвестная ошибка"
	}
	return fmt.Sprintf("HTTP %d: %s. Ответ сервера: %s", e.Status, reason, e.Body)
}

func get(path string, query url.Values) ([]byte, error) {
	endpoint := baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Accept", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, &APIError{Status: response.StatusCode, Body: string(body)}
	}
	return body, nil
}

// ── Обёртки над эндпоинтами ──────────────────────────────────────────────────

// Suggest — автодополнение: быстрые подсказки по префиксу.
func Suggest(query string, limit int) (*SuggestResponse, error) {
	body, err := get("/api/Gar/suggest", url.Values{
		"query": {query},
		"limit": {strconv.Itoa(limit)},
	})
	if err != nil {
		return nil, err
	}
	var result SuggestResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Search — полный поиск: с ОКТМО, ОКАТО, почтовым индексом и кодом региона.
func Search(query string, limit int) (*SearchResponse, error) {
	body, err := get("/api/Gar/search", url.Values{
		"query": {query},
		"limit": {strconv.Itoa(limit)},
	})
	if err != nil {
		return nil, err
	}
	var result SearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetObject — карточка объекта по GUID ФИАС: иерархия + параметры (индекс, ОКТМО,
// ОКАТО, код ИФНС, код КЛАДР).
func GetObject(guid string) (*AddressObject, error) {
	body, err := get("/api/Gar/object/"+url.PathEscape(guid), nil)
	if err != nil {
		return nil, err
	}
	var object AddressObject
	if err := json.Unmarshal(body, &object); err != nil {
		return nil, err
	}
	return &object, nil
}

// GetObjectByID — карточка объекта по числовому objectId.
func GetObjectByID(objectID int64) (*AddressObject, error) {
	body, err := get("/api/Gar/object/id/"+strconv.FormatInt(objectID, 10), nil)
	if err != nil {
		return nil, err
	}
	var object AddressObject
	if err := json.Unmarshal(body, &object); err != nil {
		return nil, err
	}
	return &object, nil
}

// GetHierarchy — путь объекта от региона до дома (сырой JSON).
func GetHierarchy(objectID int64) ([]byte, error) {
	return get("/api/Gar/hierarchy/"+strconv.FormatInt(objectID, 10), nil)
}

// GetChildren — дочерние объекты: улицы региона, дома улицы и т.д. (сырой JSON).
func GetChildren(parentObjectID int64, limit int) ([]byte, error) {
	return get("/api/Gar/children/"+strconv.FormatInt(parentObjectID, 10),
		url.Values{"limit": {strconv.Itoa(limit)}})
}

// GetRegions — список регионов РФ (сырой JSON).
func GetRegions() ([]byte, error) { return get("/api/Gar/regions", nil) }

// GetStats — статистика адресной базы (сырой JSON).
func GetStats() ([]byte, error) { return get("/api/Gar/stats", nil) }

// GetRegionStats — статистика по конкретному региону (сырой JSON).
func GetRegionStats(regionCode string) ([]byte, error) {
	return get("/api/Gar/region/"+url.PathEscape(regionCode)+"/stats", nil)
}

// ── Применение данных: автодополнение адреса в форме ──────────────────────────
// Ответ API сам по себе — просто JSON. Ценность появляется, когда из него собирают
// то, что реально уезжает в базу. Ниже — ровно то, что делает связка «фронтенд +
// бэкенд» в форме заказа или регистрации:
//
//	Шаг 1 (фронтенд): по префиксу, который набрал пользователь, дёргаем /suggest и
//	        показываем выпадающий список подсказок.
//	Шаг 2 (бэкенд): у ВЫБРАННОЙ подсказки берём objectGuid и дёргаем /object/{guid} —
//	        получаем канонический адрес и коды.
//
// ГЛАВНОЕ: в БД нельзя хранить строку, которую набрал пользователь. Хранить надо
// objectGuid (код ФИАС) — стабильный идентификатор, — а рядом денормализованно
// fullAddress, индекс, ОКТМО, ОКАТО, ИФНС, КЛАДР. Тогда «ул. Лесная», «улица Лесная»
// и «Лесная ул.» — это один objectGuid, а не три разных адреса в базе.

// CodePair — код из parameters, готовый к сохранению рядом с GUID.
type CodePair struct {
	Name  string
	Value string
}

// NormalizedAddress — карточка, готовая к сохранению в БД.
type NormalizedAddress struct {
	// ObjectGUID — первичный ключ адреса. Именно он хранится в базе.
	ObjectGUID  string
	ObjectID    int64
	FullAddress string
	ObjectType  string
	RegionCode  string
	Hierarchy   []string
	Codes       []CodePair // индекс, ОКТМО, ОКАТО, код ИФНС, код КЛАДР
}

// SuggestAddress — автодополнение адреса, доведённое до конца: подсказки →
// канонический объект. Возвращает nil, если подсказок нет: такого адреса нет в
// ГАР — вероятно опечатка или новостройка, ещё не внесённая в реестр.
func SuggestAddress(prefix string, choice int) (*NormalizedAddress, error) {
	// Шаг 1. То, что видит пользователь: выпадающий список под полем ввода.
	response, err := Suggest(prefix, 7)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Ввод пользователя: «%s»\n\n", prefix)
	fmt.Println("Подсказки (выпадающий список под полем ввода):")
	if len(response.Items) == 0 {
		fmt.Println("  — пусто")
		return nil, nil
	}

	for index, item := range response.Items {
		marker := " "
		if index+1 == choice {
			marker = ">"
		}
		fmt.Printf("  %s %d. %s  [%s]\n", marker, index+1, item.Text, item.ObjectType)
	}
	fmt.Printf("\nВсего подсказок: %d · %d мс\n", response.Count, response.ElapsedMs)

	if choice < 1 || choice > len(response.Items) {
		fmt.Printf("\nПодсказки №%d нет — беру первую.\n", choice)
		choice = 1
	}
	picked := response.Items[choice-1]

	// Шаг 2. То, что сохраняет бэкенд. Из подсказки нам нужен ТОЛЬКО objectGuid —
	// текст подсказки не хранится, он лишь помог пользователю выбрать объект.
	card, err := GetObject(picked.ObjectGUID)
	if err != nil {
		return nil, err
	}

	address := &NormalizedAddress{
		ObjectGUID:  card.ObjectGUID,
		ObjectID:    card.ObjectID,
		FullAddress: card.FullAddress,
		ObjectType:  card.ObjectType,
		RegionCode:  card.RegionCode,
	}
	for _, level := range card.Hierarchy {
		address.Hierarchy = append(address.Hierarchy, level.DisplayName)
	}
	// parameters — массив пар typeName/value. Это и есть «нормализованный адрес для
	// БД»: индекс и ОКТМО не надо спрашивать у пользователя, они приезжают сами;
	// код ИФНС и ОКТМО нужны для налоговой отчётности и госформ.
	for _, parameter := range card.Parameters {
		address.Codes = append(address.Codes, CodePair{Name: parameter.TypeName, Value: parameter.Value})
	}
	return address, nil
}

func main() {
	if apiKey == SandboxKey {
		fmt.Println("Демо-ключ: ответы сгенерированы (моки), не реальные данные.")
		fmt.Println()
	}

	prefix := "москва тверская"
	if len(os.Args) > 1 {
		prefix = os.Args[1]
	}
	choice := 1
	if len(os.Args) > 2 {
		if parsed, err := strconv.Atoi(os.Args[2]); err == nil {
			choice = parsed
		}
	}

	address, err := SuggestAddress(prefix, choice)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Ошибка:", err)
		os.Exit(1)
	}

	if address == nil {
		fmt.Println("\nПодсказок не найдено: адреса нет в ГАР.")
		fmt.Println("Вероятно, опечатка — или новостройка, ещё не внесённая в реестр.")
		return
	}

	fmt.Println("\n── Карточка для сохранения в БД ──────────────────────────────")
	fmt.Printf("  objectGuid (код ФИАС): %s   ← первичный ключ адреса\n", address.ObjectGUID)
	fmt.Printf("  objectId:              %d\n", address.ObjectID)
	fmt.Printf("  Полный адрес:          %s\n", address.FullAddress)
	fmt.Printf("  Тип объекта:           %s\n", address.ObjectType)
	fmt.Printf("  Код региона:           %s\n", address.RegionCode)

	fmt.Println("\n  Иерархия (регион → город → улица → дом):")
	for index, name := range address.Hierarchy {
		fmt.Printf("    %d. %s\n", index+1, name)
	}

	if len(address.Codes) > 0 {
		fmt.Println("\n  Коды (хранить денормализованно рядом с GUID):")
		for _, code := range address.Codes {
			fmt.Printf("    %-18s %s\n", code.Name, code.Value)
		}
	}

	fmt.Println("\nАдрес нормализован. В БД уходит objectGuid, а не строка пользователя:")
	fmt.Println("«ул. Лесная», «улица Лесная» и «Лесная ул.» — это один и тот же objectGuid.")
}
