// Клиент API ГАР/ФИАС Atlorium — поиск и нормализация российских адресов.
//
// Запуск (работает сразу, без регистрации — на демо-ключе):
//     dotnet run -- "москва тверская"
//
// Второй аргумент — номер выбранной подсказки (по умолчанию 1):
//     dotnet run -- "москва тверская" 2
//
// Боевой ключ: получить на https://atlorium.com и положить в переменную окружения
// ATLORIUM_API_KEY. Код при этом не меняется.

using System.Net;
using System.Net.Http.Headers;
using System.Text.Json;
using System.Text.Json.Serialization;

// Публичный демо-ключ. С ним API отвечает правдоподобными МОКАМИ (не реальными
// данными ГАР) — чтобы можно было встроить и протестировать интеграцию до оплаты.
// Ответы детерминированы: один и тот же запрос всегда даёт один и тот же результат,
// поэтому на них можно писать стабильные тесты.
const string SandboxKey = "ak_sandbox_demo_mockdata_v1";

var apiKey = Environment.GetEnvironmentVariable("ATLORIUM_API_KEY") ?? SandboxKey;
var baseUrl = Environment.GetEnvironmentVariable("ATLORIUM_BASE_URL") ?? "https://atlorium.com";

Console.OutputEncoding = System.Text.Encoding.UTF8;

using var http = new HttpClient
{
    BaseAddress = new Uri(baseUrl),
    Timeout = TimeSpan.FromSeconds(30),
};
http.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", apiKey);
http.DefaultRequestHeaders.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));

var client = new GarClient(http);

if (apiKey == SandboxKey)
{
    Console.WriteLine("Демо-ключ: ответы сгенерированы (моки), не реальные данные.\n");
}

var prefix = args.Length > 0 ? args[0] : "москва тверская";
var choice = args.Length > 1 && int.TryParse(args[1], out var parsed) ? parsed : 1;

NormalizedAddress? address;
try
{
    address = await AddressSuggestion.SuggestAddressAsync(client, prefix, choice);
}
catch (AtloriumException error)
{
    Console.Error.WriteLine($"Ошибка: {error.Message}");
    return 1;
}

if (address is null)
{
    Console.WriteLine("\nПодсказок не найдено: адреса нет в ГАР.");
    Console.WriteLine("Вероятно, опечатка — или новостройка, ещё не внесённая в реестр.");
    return 0;
}

Console.WriteLine("\n── Карточка для сохранения в БД ──────────────────────────────");
Console.WriteLine($"  objectGuid (код ФИАС): {address.ObjectGuid}   ← первичный ключ адреса");
Console.WriteLine($"  objectId:              {address.ObjectId}");
Console.WriteLine($"  Полный адрес:          {address.FullAddress}");
Console.WriteLine($"  Тип объекта:           {address.ObjectType}");
Console.WriteLine($"  Код региона:           {address.RegionCode}");

Console.WriteLine("\n  Иерархия (регион → город → улица → дом):");
for (var level = 0; level < address.Hierarchy.Count; level++)
{
    Console.WriteLine($"    {level + 1}. {address.Hierarchy[level]}");
}

if (address.Codes.Count > 0)
{
    Console.WriteLine("\n  Коды (хранить денормализованно рядом с GUID):");
    foreach (var code in address.Codes)
    {
        Console.WriteLine($"    {code.Name,-18} {code.Value}");
    }
}

Console.WriteLine("\nАдрес нормализован. В БД уходит objectGuid, а не строка пользователя:");
Console.WriteLine("«ул. Лесная», «улица Лесная» и «Лесная ул.» — это один и тот же objectGuid.");
return 0;

// ── Клиент ───────────────────────────────────────────────────────────────────

/// <summary>Ошибка API: HTTP-код разложен в человекочитаемую причину.</summary>
public sealed class AtloriumException(HttpStatusCode status, string body)
    : Exception($"HTTP {(int)status}: {Explain(status)}. Ответ сервера: {body[..Math.Min(200, body.Length)]}")
{
    public HttpStatusCode Status { get; } = status;

    private static string Explain(HttpStatusCode status) => (int)status switch
    {
        400 => "Неверный запрос (пустая строка поиска или некорректный GUID/objectId)",
        401 => "API-ключ отсутствует, просрочен или недействителен",
        402 => "Недостаточно кредитов на балансе — пополните на https://atlorium.com",
        404 => "Адресный объект не найден в ГАР/ФИАС",
        429 => "Превышен лимит запросов — повторите позже",
        500 => "Внутренняя ошибка при обращении к адресной базе (за сбой на своей стороне мы не списываем деньги)",
        503 => "Сервис временно недоступен (плановые работы) — повторите позже",
        _ => "Неизвестная ошибка",
    };
}

public sealed class GarClient(HttpClient http)
{
    private static readonly JsonSerializerOptions JsonOptions = new(JsonSerializerDefaults.Web);

    /// <summary>Автодополнение: быстрые подсказки по префиксу. Элементы лежат в <c>items</c>.</summary>
    public Task<SuggestResponse> SuggestAsync(string query, int limit = 7)
        => GetAsync<SuggestResponse>($"/api/Gar/suggest?query={Uri.EscapeDataString(query)}&limit={limit}");

    /// <summary>Полный поиск: с ОКТМО, ОКАТО, индексом. Элементы лежат в <c>results</c>.</summary>
    public Task<SearchResponse> SearchAsync(string query, int limit = 10)
        => GetAsync<SearchResponse>($"/api/Gar/search?query={Uri.EscapeDataString(query)}&limit={limit}");

    /// <summary>Карточка объекта по GUID ФИАС: иерархия + параметры (индекс, ОКТМО, ОКАТО, ИФНС, КЛАДР).</summary>
    public Task<AddressObject> GetObjectAsync(string guid)
        => GetAsync<AddressObject>($"/api/Gar/object/{Uri.EscapeDataString(guid)}");

    /// <summary>Карточка объекта по числовому objectId.</summary>
    public Task<AddressObject> GetObjectByIdAsync(long objectId)
        => GetAsync<AddressObject>($"/api/Gar/object/id/{objectId}");

    /// <summary>Путь объекта от региона до дома (сырой JSON).</summary>
    public Task<string> GetHierarchyAsync(long objectId)
        => GetRawAsync($"/api/Gar/hierarchy/{objectId}");

    /// <summary>Дочерние объекты: улицы региона, дома улицы и т.д. (сырой JSON).</summary>
    public Task<string> GetChildrenAsync(long parentObjectId, int limit = 50)
        => GetRawAsync($"/api/Gar/children/{parentObjectId}?limit={limit}");

    /// <summary>Список регионов РФ (сырой JSON).</summary>
    public Task<string> GetRegionsAsync() => GetRawAsync("/api/Gar/regions");

    /// <summary>Статистика адресной базы (сырой JSON).</summary>
    public Task<string> GetStatsAsync() => GetRawAsync("/api/Gar/stats");

    /// <summary>Статистика по конкретному региону (сырой JSON).</summary>
    public Task<string> GetRegionStatsAsync(string regionCode)
        => GetRawAsync($"/api/Gar/region/{Uri.EscapeDataString(regionCode)}/stats");

    private async Task<T> GetAsync<T>(string path)
    {
        var json = await GetRawAsync(path);
        return JsonSerializer.Deserialize<T>(json, JsonOptions)
               ?? throw new InvalidOperationException("Пустой ответ API.");
    }

    private async Task<string> GetRawAsync(string path)
    {
        using var response = await http.GetAsync(path);
        var body = await response.Content.ReadAsStringAsync();
        if (!response.IsSuccessStatusCode)
        {
            throw new AtloriumException(response.StatusCode, body);
        }
        return body;
    }
}

// ── Модель ответа ────────────────────────────────────────────────────────────

/// <summary>Подсказка автодополнения.</summary>
public sealed record SuggestItem
{
    public string Text { get; init; } = "";
    public long ObjectId { get; init; }

    /// <summary>GUID ФИАС — стабильный идентификатор адреса. Именно он хранится в БД.</summary>
    public string ObjectGuid { get; init; } = "";

    public string ObjectType { get; init; } = "";
}

public sealed record SuggestResponse
{
    public string Query { get; init; } = "";
    public IReadOnlyList<SuggestItem> Items { get; init; } = [];
    public int Count { get; init; }
    public int ElapsedMs { get; init; }
}

/// <summary>Результат полного поиска.</summary>
public sealed record SearchResult
{
    public string FullAddress { get; init; } = "";
    public string ObjectType { get; init; } = "";
    public string ObjectGuid { get; init; } = "";
    public long ObjectId { get; init; }
    public string? RegionCode { get; init; }
    public string? PostalCode { get; init; }
    public string? Oktmo { get; init; }
    public string? Okato { get; init; }
}

public sealed record SearchResponse
{
    public string Query { get; init; } = "";
    public IReadOnlyList<SearchResult> Results { get; init; } = [];
    public int Count { get; init; }
    public int ElapsedMs { get; init; }
}

/// <summary>Уровень иерархии: регион, город, улица, дом.</summary>
public sealed record HierarchyLevel(
    [property: JsonPropertyName("objectId")] long ObjectId,
    [property: JsonPropertyName("displayName")] string DisplayName);

/// <summary>Пара «тип — значение»: почтовый индекс, ОКТМО, ОКАТО, код ИФНС, код КЛАДР.</summary>
public sealed record AddressParameter(
    [property: JsonPropertyName("typeId")] int TypeId,
    [property: JsonPropertyName("typeName")] string TypeName,
    [property: JsonPropertyName("value")] string Value);

/// <summary>Карточка адресного объекта ГАР/ФИАС.</summary>
public sealed record AddressObject
{
    public long ObjectId { get; init; }
    public string ObjectGuid { get; init; } = "";
    public string FullAddress { get; init; } = "";
    public string ObjectType { get; init; } = "";
    public string RegionCode { get; init; } = "";
    public IReadOnlyList<HierarchyLevel> Hierarchy { get; init; } = [];
    public IReadOnlyList<AddressParameter> Parameters { get; init; } = [];
    public int ElapsedMs { get; init; }
}

// ── Применение данных: автодополнение адреса в форме ──────────────────────────
// Ответ API сам по себе — просто JSON. Ценность появляется, когда из него собирают
// то, что реально уезжает в базу. Ниже — ровно то, что делает связка «фронтенд +
// бэкенд» в форме заказа или регистрации:
//
//   Шаг 1 (фронтенд): по префиксу, который набрал пользователь, дёргаем /suggest и
//           показываем выпадающий список подсказок.
//   Шаг 2 (бэкенд): у ВЫБРАННОЙ подсказки берём objectGuid и дёргаем /object/{guid} —
//           получаем канонический адрес и коды.
//
// ГЛАВНОЕ: в БД нельзя хранить строку, которую набрал пользователь. Хранить надо
// objectGuid (код ФИАС) — стабильный идентификатор, — а рядом денормализованно
// fullAddress, индекс, ОКТМО, ОКАТО, ИФНС, КЛАДР. Тогда «ул. Лесная», «улица Лесная»
// и «Лесная ул.» — это один objectGuid, а не три разных адреса в базе.

/// <summary>Код из parameters, готовый к сохранению рядом с GUID.</summary>
public sealed record CodePair(string Name, string Value);

/// <summary>Карточка, готовая к сохранению в БД.</summary>
public sealed record NormalizedAddress
{
    /// <summary>Первичный ключ адреса. Именно он хранится в базе.</summary>
    public required string ObjectGuid { get; init; }

    public required long ObjectId { get; init; }
    public required string FullAddress { get; init; }
    public required string ObjectType { get; init; }
    public required string RegionCode { get; init; }
    public IReadOnlyList<string> Hierarchy { get; init; } = [];
    public IReadOnlyList<CodePair> Codes { get; init; } = [];
}

public static class AddressSuggestion
{
    /// <summary>
    /// Автодополнение адреса, доведённое до конца: подсказки → канонический объект.
    /// Возвращает null, если подсказок нет: такого адреса нет в ГАР — вероятно
    /// опечатка или новостройка, ещё не внесённая в реестр.
    /// </summary>
    public static async Task<NormalizedAddress?> SuggestAddressAsync(GarClient client, string prefix, int choice = 1)
    {
        // Шаг 1. То, что видит пользователь: выпадающий список под полем ввода.
        var response = await client.SuggestAsync(prefix, limit: 7);
        var items = response.Items;

        Console.WriteLine($"Ввод пользователя: «{prefix}»\n");
        Console.WriteLine("Подсказки (выпадающий список под полем ввода):");
        if (items.Count == 0)
        {
            Console.WriteLine("  — пусто");
            return null;
        }

        for (var index = 0; index < items.Count; index++)
        {
            var marker = index + 1 == choice ? ">" : " ";
            Console.WriteLine($"  {marker} {index + 1}. {items[index].Text}  [{items[index].ObjectType}]");
        }
        Console.WriteLine($"\nВсего подсказок: {response.Count} · {response.ElapsedMs} мс");

        if (choice < 1 || choice > items.Count)
        {
            Console.WriteLine($"\nПодсказки №{choice} нет — беру первую.");
            choice = 1;
        }
        var picked = items[choice - 1];

        // Шаг 2. То, что сохраняет бэкенд. Из подсказки нам нужен ТОЛЬКО objectGuid —
        // текст подсказки не хранится, он лишь помог пользователю выбрать объект.
        var card = await client.GetObjectAsync(picked.ObjectGuid);

        return new NormalizedAddress
        {
            ObjectGuid = card.ObjectGuid,
            ObjectId = card.ObjectId,
            FullAddress = card.FullAddress,
            ObjectType = card.ObjectType,
            RegionCode = card.RegionCode,
            Hierarchy = card.Hierarchy.Select(level => level.DisplayName).ToList(),

            // parameters — массив пар typeName/value. Это и есть «нормализованный адрес
            // для БД»: индекс и ОКТМО не надо спрашивать у пользователя, они приезжают
            // сами; код ИФНС и ОКТМО нужны для налоговой отчётности и госформ.
            Codes = card.Parameters.Select(p => new CodePair(p.TypeName, p.Value)).ToList(),
        };
    }
}
