<?php

/**
 * Клиент API ГАР/ФИАС Atlorium — поиск и нормализация российских адресов.
 *
 * Запуск (работает сразу, без регистрации — на демо-ключе):
 *   php main.php "москва тверская"
 *
 * Второй аргумент — номер выбранной подсказки (по умолчанию 1):
 *   php main.php "москва тверская" 2
 *
 * Боевой ключ: получить на https://atlorium.com и положить в переменную окружения
 * ATLORIUM_API_KEY. Код при этом не меняется.
 */

declare(strict_types=1);

/**
 * Публичный демо-ключ. С ним API отвечает правдоподобными МОКАМИ (не реальными
 * данными ГАР) — чтобы можно было встроить и протестировать интеграцию до оплаты.
 * Ответы детерминированы: один и тот же запрос всегда даёт один и тот же результат,
 * поэтому на них можно писать стабильные тесты.
 */
const SANDBOX_KEY = 'ak_sandbox_demo_mockdata_v1';

const TIMEOUT = 30;

/** Ошибка API: HTTP-код разложен в человекочитаемую причину. */
final class AtloriumError extends RuntimeException
{
    private const REASONS = [
        400 => 'Неверный запрос (пустая строка поиска или некорректный GUID/objectId)',
        401 => 'API-ключ отсутствует, просрочен или недействителен',
        402 => 'Недостаточно кредитов на балансе — пополните на https://atlorium.com',
        404 => 'Адресный объект не найден в ГАР/ФИАС',
        429 => 'Превышен лимит запросов — повторите позже',
        500 => 'Внутренняя ошибка при обращении к адресной базе (за сбой на своей стороне мы не списываем деньги)',
        503 => 'Сервис временно недоступен (плановые работы) — повторите позже',
    ];

    public function __construct(public readonly int $status, string $body)
    {
        $reason = self::REASONS[$status] ?? 'Неизвестная ошибка';
        parent::__construct(sprintf(
            'HTTP %d: %s. Ответ сервера: %s',
            $status,
            $reason,
            mb_substr($body, 0, 200)
        ));
    }
}

final class GarClient
{
    private string $apiKey;
    private string $baseUrl;

    public function __construct(?string $apiKey = null, ?string $baseUrl = null)
    {
        $this->apiKey = $apiKey ?? (getenv('ATLORIUM_API_KEY') ?: SANDBOX_KEY);
        $this->baseUrl = $baseUrl ?? (getenv('ATLORIUM_BASE_URL') ?: 'https://atlorium.com');
    }

    public function isSandbox(): bool
    {
        return $this->apiKey === SANDBOX_KEY;
    }

    /**
     * @param array<string, string|int> $params
     * @return array<string, mixed>
     */
    private function get(string $path, array $params = []): array
    {
        $url = $this->baseUrl . $path;
        if ($params !== []) {
            $url .= '?' . http_build_query($params);
        }

        $curl = curl_init($url);
        curl_setopt_array($curl, [
            CURLOPT_RETURNTRANSFER => true,
            CURLOPT_TIMEOUT => TIMEOUT,
            CURLOPT_HTTPHEADER => [
                'Authorization: Bearer ' . $this->apiKey,
                'Accept: application/json',
            ],
        ]);

        $body = curl_exec($curl);
        if ($body === false) {
            $error = curl_error($curl);
            curl_close($curl);
            throw new RuntimeException("Сетевая ошибка: {$error}");
        }

        $status = curl_getinfo($curl, CURLINFO_RESPONSE_CODE);
        curl_close($curl);

        if ($status !== 200) {
            throw new AtloriumError($status, (string) $body);
        }

        return json_decode((string) $body, true, 512, JSON_THROW_ON_ERROR);
    }

    // ── Обёртки над эндпоинтами ──────────────────────────────────────────────

    /**
     * Автодополнение: быстрые подсказки по префиксу. Элементы лежат в `items`.
     *
     * @return array<string, mixed>
     */
    public function suggest(string $query, int $limit = 7): array
    {
        return $this->get('/api/Gar/suggest', ['query' => $query, 'limit' => $limit]);
    }

    /**
     * Полный поиск: с ОКТМО, ОКАТО, индексом. Элементы лежат в `results`.
     *
     * @return array<string, mixed>
     */
    public function search(string $query, int $limit = 10): array
    {
        return $this->get('/api/Gar/search', ['query' => $query, 'limit' => $limit]);
    }

    /**
     * Карточка объекта по GUID ФИАС: иерархия + параметры (индекс, ОКТМО, ОКАТО, ИФНС, КЛАДР).
     *
     * @return array<string, mixed>
     */
    public function getObject(string $guid): array
    {
        return $this->get('/api/Gar/object/' . rawurlencode($guid));
    }

    /**
     * Карточка объекта по числовому objectId.
     *
     * @return array<string, mixed>
     */
    public function getObjectById(int $objectId): array
    {
        return $this->get('/api/Gar/object/id/' . $objectId);
    }

    /**
     * Путь объекта от региона до дома.
     *
     * @return array<string, mixed>
     */
    public function getHierarchy(int $objectId): array
    {
        return $this->get('/api/Gar/hierarchy/' . $objectId);
    }

    /**
     * Дочерние объекты: улицы региона, дома улицы и т.д.
     *
     * @return array<string, mixed>
     */
    public function getChildren(int $parentObjectId, int $limit = 50): array
    {
        return $this->get('/api/Gar/children/' . $parentObjectId, ['limit' => $limit]);
    }

    /**
     * Список регионов РФ.
     *
     * @return array<string, mixed>
     */
    public function getRegions(): array
    {
        return $this->get('/api/Gar/regions');
    }

    /**
     * Статистика адресной базы.
     *
     * @return array<string, mixed>
     */
    public function getStats(): array
    {
        return $this->get('/api/Gar/stats');
    }

    /**
     * Статистика по конкретному региону.
     *
     * @return array<string, mixed>
     */
    public function getRegionStats(string $regionCode): array
    {
        return $this->get('/api/Gar/region/' . rawurlencode($regionCode) . '/stats');
    }
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

/**
 * Автодополнение адреса, доведённое до конца: подсказки → канонический объект.
 *
 * Возвращает null, если подсказок нет: такого адреса нет в ГАР — вероятно опечатка
 * или новостройка, ещё не внесённая в реестр.
 *
 * @return array{
 *     objectGuid: string, objectId: int, fullAddress: string, objectType: string,
 *     regionCode: string, hierarchy: list<string>, codes: array<string, string>
 * }|null
 */
function suggestAddress(GarClient $client, string $prefix, int $choice = 1): ?array
{
    // Шаг 1. То, что видит пользователь: выпадающий список под полем ввода.
    $response = $client->suggest($prefix, 7);
    $items = $response['items'] ?? [];

    echo "Ввод пользователя: «{$prefix}»\n\n";
    echo "Подсказки (выпадающий список под полем ввода):\n";
    if ($items === []) {
        echo "  — пусто\n";
        return null;
    }

    foreach ($items as $index => $item) {
        $marker = ($index + 1) === $choice ? '>' : ' ';
        printf("  %s %d. %s  [%s]\n", $marker, $index + 1, $item['text'], $item['objectType']);
    }
    printf("\nВсего подсказок: %d · %d мс\n", $response['count'], $response['elapsedMs']);

    if ($choice < 1 || $choice > count($items)) {
        echo "\nПодсказки №{$choice} нет — беру первую.\n";
        $choice = 1;
    }
    $picked = $items[$choice - 1];

    // Шаг 2. То, что сохраняет бэкенд. Из подсказки нам нужен ТОЛЬКО objectGuid —
    // текст подсказки не хранится, он лишь помог пользователю выбрать объект.
    $card = $client->getObject($picked['objectGuid']);

    $hierarchy = [];
    foreach ($card['hierarchy'] ?? [] as $level) {
        $hierarchy[] = $level['displayName'];
    }

    // parameters — массив пар typeName/value. Это и есть «нормализованный адрес для
    // БД»: индекс и ОКТМО не надо спрашивать у пользователя, они приезжают сами;
    // код ИФНС и ОКТМО нужны для налоговой отчётности и госформ.
    $codes = [];
    foreach ($card['parameters'] ?? [] as $parameter) {
        $codes[$parameter['typeName']] = $parameter['value'];
    }

    return [
        'objectGuid' => $card['objectGuid'],
        'objectId' => $card['objectId'],
        'fullAddress' => $card['fullAddress'],
        'objectType' => $card['objectType'] ?? '',
        'regionCode' => $card['regionCode'] ?? '',
        'hierarchy' => $hierarchy,
        'codes' => $codes,
    ];
}

/** Выравнивание с учётом кириллицы: str_pad считает байты, а не символы. */
function pad(string $value, int $width): string
{
    $length = mb_strlen($value);
    return $length >= $width ? $value : $value . str_repeat(' ', $width - $length);
}

// ── Демонстрация ─────────────────────────────────────────────────────────────

$client = new GarClient();

if ($client->isSandbox()) {
    echo "Демо-ключ: ответы сгенерированы (моки), не реальные данные.\n\n";
}

$prefix = $argv[1] ?? 'москва тверская';
$choice = isset($argv[2]) ? (int) $argv[2] : 1;

try {
    $address = suggestAddress($client, $prefix, $choice);
} catch (AtloriumError $error) {
    fwrite(STDERR, "Ошибка: {$error->getMessage()}\n");
    exit(1);
}

if ($address === null) {
    echo "\nПодсказок не найдено: адреса нет в ГАР.\n";
    echo "Вероятно, опечатка — или новостройка, ещё не внесённая в реестр.\n";
    exit(0);
}

echo "\n── Карточка для сохранения в БД ──────────────────────────────\n";
echo "  objectGuid (код ФИАС): {$address['objectGuid']}   ← первичный ключ адреса\n";
echo "  objectId:              {$address['objectId']}\n";
echo "  Полный адрес:          {$address['fullAddress']}\n";
echo "  Тип объекта:           {$address['objectType']}\n";
echo "  Код региона:           {$address['regionCode']}\n";

echo "\n  Иерархия (регион → город → улица → дом):\n";
foreach ($address['hierarchy'] as $index => $name) {
    printf("    %d. %s\n", $index + 1, $name);
}

if ($address['codes'] !== []) {
    echo "\n  Коды (хранить денормализованно рядом с GUID):\n";
    foreach ($address['codes'] as $name => $value) {
        echo '    ' . pad($name, 18) . " {$value}\n";
    }
}

echo "\nАдрес нормализован. В БД уходит objectGuid, а не строка пользователя:\n";
echo "«ул. Лесная», «улица Лесная» и «Лесная ул.» — это один и тот же objectGuid.\n";
