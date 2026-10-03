/*
 * Клиент API ГАР/ФИАС Atlorium — поиск и нормализация российских адресов.
 *
 * Запуск (работает сразу, без регистрации — на демо-ключе).
 * Начиная с Java 11 файл запускается напрямую, без компиляции и без зависимостей:
 *
 *     java Main.java "москва тверская"
 *
 * Второй аргумент — номер выбранной подсказки (по умолчанию 1):
 *
 *     java Main.java "москва тверская" 2
 *
 * Боевой ключ: получить на https://atlorium.com и положить в переменную окружения
 * ATLORIUM_API_KEY. Код при этом не меняется.
 */

import java.io.IOException;
import java.net.URI;
import java.net.URLEncoder;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

public class Main {

    /**
     * Публичный демо-ключ. С ним API отвечает правдоподобными МОКАМИ (не реальными
     * данными ГАР) — чтобы можно было встроить и протестировать интеграцию до оплаты.
     * Ответы детерминированы: один и тот же запрос всегда даёт один и тот же результат,
     * поэтому на них можно писать стабильные тесты.
     */
    static final String SANDBOX_KEY = "ak_sandbox_demo_mockdata_v1";

    static final String API_KEY = envOr("ATLORIUM_API_KEY", SANDBOX_KEY);
    static final String BASE_URL = envOr("ATLORIUM_BASE_URL", "https://atlorium.com");

    static final HttpClient CLIENT = HttpClient.newBuilder()
            .connectTimeout(Duration.ofSeconds(30))
            .build();

    static String envOr(String key, String fallback) {
        String value = System.getenv(key);
        return (value == null || value.isBlank()) ? fallback : value;
    }

    /** Ошибка API: HTTP-код разложен в человекочитаемую причину. */
    static class AtloriumException extends RuntimeException {
        private static final Map<Integer, String> REASONS = Map.of(
                400, "Неверный запрос (пустая строка поиска или некорректный GUID/objectId)",
                401, "API-ключ отсутствует, просрочен или недействителен",
                402, "Недостаточно кредитов на балансе — пополните на https://atlorium.com",
                404, "Адресный объект не найден в ГАР/ФИАС",
                429, "Превышен лимит запросов — повторите позже",
                500, "Внутренняя ошибка при обращении к адресной базе (за сбой на своей стороне мы не списываем деньги)",
                503, "Сервис временно недоступен (плановые работы) — повторите позже");

        final int status;

        AtloriumException(int status, String body) {
            super("HTTP " + status + ": "
                    + REASONS.getOrDefault(status, "Неизвестная ошибка")
                    + ". Ответ сервера: " + body.substring(0, Math.min(200, body.length())));
            this.status = status;
        }
    }

    static String get(String path, String query) throws IOException, InterruptedException {
        String url = BASE_URL + path + (query.isEmpty() ? "" : "?" + query);

        HttpRequest request = HttpRequest.newBuilder(URI.create(url))
                .header("Authorization", "Bearer " + API_KEY)
                .header("Accept", "application/json")
                .timeout(Duration.ofSeconds(30))
                .GET()
                .build();

        HttpResponse<byte[]> response = CLIENT.send(request, HttpResponse.BodyHandlers.ofByteArray());
        String body = new String(response.body(), StandardCharsets.UTF_8);
        if (response.statusCode() != 200) {
            throw new AtloriumException(response.statusCode(), body);
        }
        return body;
    }

    static String encode(String value) {
        return URLEncoder.encode(value, StandardCharsets.UTF_8);
    }

    // ── Обёртки над эндпоинтами ──────────────────────────────────────────────

    /** Автодополнение: быстрые подсказки по префиксу. Элементы лежат в поле items. */
    static String suggest(String query, int limit) throws IOException, InterruptedException {
        return get("/api/Gar/suggest", "query=" + encode(query) + "&limit=" + limit);
    }

    /** Полный поиск: с ОКТМО, ОКАТО, почтовым индексом. Элементы лежат в поле results. */
    static String search(String query, int limit) throws IOException, InterruptedException {
        return get("/api/Gar/search", "query=" + encode(query) + "&limit=" + limit);
    }

    /** Карточка объекта по GUID ФИАС: иерархия + параметры (индекс, ОКТМО, ОКАТО, ИФНС, КЛАДР). */
    static String getObject(String guid) throws IOException, InterruptedException {
        return get("/api/Gar/object/" + encode(guid), "");
    }

    /** Карточка объекта по числовому objectId. */
    static String getObjectById(long objectId) throws IOException, InterruptedException {
        return get("/api/Gar/object/id/" + objectId, "");
    }

    /** Путь объекта от региона до дома. */
    static String getHierarchy(long objectId) throws IOException, InterruptedException {
        return get("/api/Gar/hierarchy/" + objectId, "");
    }

    /** Дочерние объекты: улицы региона, дома улицы и т.д. */
    static String getChildren(long parentObjectId, int limit) throws IOException, InterruptedException {
        return get("/api/Gar/children/" + parentObjectId, "limit=" + limit);
    }

    /** Список регионов РФ. */
    static String getRegions() throws IOException, InterruptedException {
        return get("/api/Gar/regions", "");
    }

    /** Статистика адресной базы. */
    static String getStats() throws IOException, InterruptedException {
        return get("/api/Gar/stats", "");
    }

    /** Статистика по конкретному региону. */
    static String getRegionStats(String regionCode) throws IOException, InterruptedException {
        return get("/api/Gar/region/" + encode(regionCode) + "/stats", "");
    }

    // ── Разбор JSON ──────────────────────────────────────────────────────────
    // Пример намеренно оставлен без внешних зависимостей, чтобы запускаться одной
    // командой `java Main.java`. В рабочем проекте берите Jackson или Gson и
    // маппьте ответ в полноценную запись — эти регулярки и ручной разбор скобок
    // существуют только ради отсутствия pom.xml.

    /** Значение строкового поля верхнего уровня. */
    static String str(String json, String field) {
        Matcher matcher = Pattern.compile("\"" + field + "\"\\s*:\\s*\"((?:[^\"\\\\]|\\\\.)*)\"").matcher(json);
        return matcher.find() ? matcher.group(1).replace("\\\"", "\"") : null;
    }

    /** Значение числового поля верхнего уровня. */
    static String number(String json, String field) {
        Matcher matcher = Pattern.compile("\"" + field + "\"\\s*:\\s*(-?\\d+)").matcher(json);
        return matcher.find() ? matcher.group(1) : null;
    }

    /**
     * Разбивает массив объектов на элементы: "field":[{...},{...}] → список строк "{...}".
     * Считаем скобки вручную, чтобы не разорвать вложенные объекты.
     */
    static List<String> objects(String json, String field) {
        List<String> items = new ArrayList<>();
        int start = json.indexOf("\"" + field + "\"");
        if (start < 0) {
            return items;
        }
        int bracket = json.indexOf('[', start);
        if (bracket < 0) {
            return items;
        }

        int depth = 0;
        int itemStart = -1;
        for (int i = bracket; i < json.length(); i++) {
            char symbol = json.charAt(i);
            if (symbol == '{') {
                if (depth == 0) {
                    itemStart = i;
                }
                depth++;
            } else if (symbol == '}') {
                depth--;
                if (depth == 0 && itemStart >= 0) {
                    items.add(json.substring(itemStart, i + 1));
                    itemStart = -1;
                }
            } else if (symbol == ']' && depth == 0) {
                break;
            }
        }
        return items;
    }

    // ── Применение данных: автодополнение адреса в форме ──────────────────────
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

    /** Код из parameters, готовый к сохранению рядом с GUID. */
    record CodePair(String name, String value) {}

    /** Карточка, готовая к сохранению в БД. */
    record NormalizedAddress(
            /* objectGuid — первичный ключ адреса. Именно он хранится в базе. */
            String objectGuid,
            String objectId,
            String fullAddress,
            String objectType,
            String regionCode,
            List<String> hierarchy,
            List<CodePair> codes) {}

    /**
     * Автодополнение адреса, доведённое до конца: подсказки → канонический объект.
     * Возвращает null, если подсказок нет: такого адреса нет в ГАР — вероятно
     * опечатка или новостройка, ещё не внесённая в реестр.
     */
    static NormalizedAddress suggestAddress(String prefix, int choice)
            throws IOException, InterruptedException {

        // Шаг 1. То, что видит пользователь: выпадающий список под полем ввода.
        String response = suggest(prefix, 7);
        List<String> items = objects(response, "items");

        System.out.printf("Ввод пользователя: «%s»%n%n", prefix);
        System.out.println("Подсказки (выпадающий список под полем ввода):");
        if (items.isEmpty()) {
            System.out.println("  — пусто");
            return null;
        }

        for (int i = 0; i < items.size(); i++) {
            String marker = (i + 1 == choice) ? ">" : " ";
            System.out.printf("  %s %d. %s  [%s]%n",
                    marker, i + 1, str(items.get(i), "text"), str(items.get(i), "objectType"));
        }
        System.out.printf("%nВсего подсказок: %s · %s мс%n",
                number(response, "count"), number(response, "elapsedMs"));

        if (choice < 1 || choice > items.size()) {
            System.out.printf("%nПодсказки №%d нет — беру первую.%n", choice);
            choice = 1;
        }
        String picked = items.get(choice - 1);

        // Шаг 2. То, что сохраняет бэкенд. Из подсказки нам нужен ТОЛЬКО objectGuid —
        // текст подсказки не хранится, он лишь помог пользователю выбрать объект.
        String card = getObject(str(picked, "objectGuid"));

        List<String> hierarchy = new ArrayList<>();
        for (String level : objects(card, "hierarchy")) {
            hierarchy.add(str(level, "displayName"));
        }

        // parameters — массив пар typeName/value. Это и есть «нормализованный адрес
        // для БД»: индекс и ОКТМО не надо спрашивать у пользователя, они приезжают
        // сами; код ИФНС и ОКТМО нужны для налоговой отчётности и госформ.
        List<CodePair> codes = new ArrayList<>();
        for (String parameter : objects(card, "parameters")) {
            codes.add(new CodePair(str(parameter, "typeName"), str(parameter, "value")));
        }

        return new NormalizedAddress(
                str(card, "objectGuid"),
                number(card, "objectId"),
                str(card, "fullAddress"),
                str(card, "objectType"),
                str(card, "regionCode"),
                hierarchy,
                codes);
    }

    public static void main(String[] args) throws Exception {
        if (API_KEY.equals(SANDBOX_KEY)) {
            System.out.println("Демо-ключ: ответы сгенерированы (моки), не реальные данные.\n");
        }

        String prefix = args.length > 0 ? args[0] : "москва тверская";
        int choice = args.length > 1 ? Integer.parseInt(args[1]) : 1;

        NormalizedAddress address;
        try {
            address = suggestAddress(prefix, choice);
        } catch (AtloriumException error) {
            System.err.println("Ошибка: " + error.getMessage());
            System.exit(1);
            return;
        }

        if (address == null) {
            System.out.println("\nПодсказок не найдено: адреса нет в ГАР.");
            System.out.println("Вероятно, опечатка — или новостройка, ещё не внесённая в реестр.");
            return;
        }

        System.out.println("\n── Карточка для сохранения в БД ──────────────────────────────");
        System.out.println("  objectGuid (код ФИАС): " + address.objectGuid() + "   ← первичный ключ адреса");
        System.out.println("  objectId:              " + address.objectId());
        System.out.println("  Полный адрес:          " + address.fullAddress());
        System.out.println("  Тип объекта:           " + address.objectType());
        System.out.println("  Код региона:           " + address.regionCode());

        System.out.println("\n  Иерархия (регион → город → улица → дом):");
        for (int i = 0; i < address.hierarchy().size(); i++) {
            System.out.printf("    %d. %s%n", i + 1, address.hierarchy().get(i));
        }

        if (!address.codes().isEmpty()) {
            System.out.println("\n  Коды (хранить денормализованно рядом с GUID):");
            for (CodePair code : address.codes()) {
                System.out.printf("    %-18s %s%n", code.name(), code.value());
            }
        }

        System.out.println("\nАдрес нормализован. В БД уходит objectGuid, а не строка пользователя:");
        System.out.println("«ул. Лесная», «улица Лесная» и «Лесная ул.» — это один и тот же objectGuid.");
    }
}
