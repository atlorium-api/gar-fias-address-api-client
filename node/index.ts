/**
 * Клиент API ГАР/ФИАС Atlorium — поиск и нормализация российских адресов.
 *
 * Запуск (работает сразу, без регистрации — на демо-ключе):
 *   npm install
 *   npm start -- "москва тверская"
 *
 * Второй аргумент — номер выбранной подсказки (по умолчанию 1):
 *   npm start -- "москва тверская" 2
 *
 * Боевой ключ: получить на https://atlorium.com и положить в переменную окружения
 * ATLORIUM_API_KEY. Код при этом не меняется.
 */

/**
 * Публичный демо-ключ. С ним API отвечает правдоподобными МОКАМИ (не реальными
 * данными ГАР) — чтобы можно было встроить и протестировать интеграцию до оплаты.
 * Ответы детерминированы: один и тот же запрос всегда даёт один и тот же результат,
 * поэтому на них можно писать стабильные тесты.
 */
const SANDBOX_KEY = 'ak_sandbox_demo_mockdata_v1';

const API_KEY = process.env.ATLORIUM_API_KEY ?? SANDBOX_KEY;
const BASE_URL = process.env.ATLORIUM_BASE_URL ?? 'https://atlorium.com';

const TIMEOUT_MS = 30_000;

// ── Модель ответа ────────────────────────────────────────────────────────────

/** Подсказка автодополнения. Элементы /suggest лежат в поле `items`. */
export interface SuggestItem {
  text: string;
  objectId: number;
  /** GUID ФИАС — стабильный идентификатор адреса. Именно он хранится в БД. */
  objectGuid: string;
  objectType: string;
}

export interface SuggestResponse {
  query: string;
  items: SuggestItem[];
  count: number;
  elapsedMs: number;
}

/** Результат полного поиска. Элементы /search лежат в поле `results`. */
export interface SearchResult {
  fullAddress: string;
  objectType: string;
  objectGuid: string;
  objectId: number;
  regionCode: string;
  postalCode: string;
  oktmo: string;
  okato: string;
}

export interface SearchResponse {
  query: string;
  results: SearchResult[];
  count: number;
  elapsedMs: number;
}

/** Уровень иерархии: регион, город, улица, дом. */
export interface HierarchyLevel {
  objectId: number;
  displayName: string;
}

/** Пара «тип — значение»: почтовый индекс, ОКТМО, ОКАТО, код ИФНС, код КЛАДР. */
export interface AddressParameter {
  typeId: number;
  typeName: string;
  value: string;
}

/** Карточка адресного объекта ГАР/ФИАС. */
export interface AddressObject {
  objectId: number;
  objectGuid: string;
  fullAddress: string;
  objectType: string;
  regionCode: string;
  hierarchy: HierarchyLevel[];
  parameters: AddressParameter[];
  elapsedMs: number;
}

const ERROR_REASONS: Record<number, string> = {
  400: 'Неверный запрос (пустая строка поиска или некорректный GUID/objectId)',
  401: 'API-ключ отсутствует, просрочен или недействителен',
  402: 'Недостаточно кредитов на балансе — пополните на https://atlorium.com',
  404: 'Адресный объект не найден в ГАР/ФИАС',
  429: 'Превышен лимит запросов — повторите позже',
  500: 'Внутренняя ошибка при обращении к адресной базе (за сбой на своей стороне мы не списываем деньги)',
  503: 'Сервис временно недоступен (плановые работы) — повторите позже',
};

/** Ошибка API: HTTP-код разложен в человекочитаемую причину. */
export class AtloriumError extends Error {
  constructor(readonly status: number, body: string) {
    const reason = ERROR_REASONS[status] ?? 'Неизвестная ошибка';
    super(`HTTP ${status}: ${reason}. Ответ сервера: ${body.slice(0, 200)}`);
    this.name = 'AtloriumError';
  }
}

async function request<T>(path: string, params: Record<string, string> = {}): Promise<T> {
  const url = new URL(path, BASE_URL);
  for (const [key, value] of Object.entries(params)) {
    url.searchParams.set(key, value);
  }

  const response = await fetch(url, {
    headers: {
      Authorization: `Bearer ${API_KEY}`,
      Accept: 'application/json',
    },
    signal: AbortSignal.timeout(TIMEOUT_MS),
  });

  if (!response.ok) {
    throw new AtloriumError(response.status, await response.text());
  }
  return response.json() as Promise<T>;
}

// ── Обёртки над эндпоинтами ──────────────────────────────────────────────────

/** Автодополнение: быстрые подсказки по префиксу. */
export function suggest(query: string, limit = 7): Promise<SuggestResponse> {
  return request<SuggestResponse>('/api/Gar/suggest', { query, limit: String(limit) });
}

/** Полный поиск: с ОКТМО, ОКАТО, почтовым индексом и кодом региона. */
export function search(query: string, limit = 10): Promise<SearchResponse> {
  return request<SearchResponse>('/api/Gar/search', { query, limit: String(limit) });
}

/** Карточка объекта по GUID ФИАС: иерархия + параметры (индекс, ОКТМО, ОКАТО, ИФНС, КЛАДР). */
export function getObject(guid: string): Promise<AddressObject> {
  return request<AddressObject>(`/api/Gar/object/${encodeURIComponent(guid)}`);
}

/** Карточка объекта по числовому objectId. */
export function getObjectById(objectId: number): Promise<AddressObject> {
  return request<AddressObject>(`/api/Gar/object/id/${objectId}`);
}

/** Путь объекта от региона до дома. */
export function getHierarchy(objectId: number): Promise<unknown> {
  return request<unknown>(`/api/Gar/hierarchy/${objectId}`);
}

/** Дочерние объекты: улицы региона, дома улицы и т.д. */
export function getChildren(parentObjectId: number, limit = 50): Promise<unknown> {
  return request<unknown>(`/api/Gar/children/${parentObjectId}`, { limit: String(limit) });
}

/** Список регионов РФ. */
export function getRegions(): Promise<unknown> {
  return request<unknown>('/api/Gar/regions');
}

/** Статистика адресной базы. */
export function getStats(): Promise<unknown> {
  return request<unknown>('/api/Gar/stats');
}

/** Статистика по конкретному региону. */
export function getRegionStats(regionCode: string): Promise<unknown> {
  return request<unknown>(`/api/Gar/region/${encodeURIComponent(regionCode)}/stats`);
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

/** Карточка, готовая к сохранению в БД. */
export interface NormalizedAddress {
  /** Первичный ключ адреса. Именно он хранится в базе. */
  objectGuid: string;
  objectId: number;
  fullAddress: string;
  objectType: string;
  regionCode: string;
  hierarchy: string[];
  /** Индекс, ОКТМО, ОКАТО, код ИФНС, код КЛАДР. */
  codes: Record<string, string>;
}

/**
 * Автодополнение адреса, доведённое до конца: подсказки → канонический объект.
 * Возвращает null, если подсказок нет: такого адреса нет в ГАР — вероятно опечатка
 * или новостройка, ещё не внесённая в реестр.
 */
export async function suggestAddress(prefix: string, choice = 1): Promise<NormalizedAddress | null> {
  // Шаг 1. То, что видит пользователь: выпадающий список под полем ввода.
  const response = await suggest(prefix, 7);
  const items = response.items ?? [];

  console.log(`Ввод пользователя: «${prefix}»\n`);
  console.log('Подсказки (выпадающий список под полем ввода):');
  if (items.length === 0) {
    console.log('  — пусто');
    return null;
  }

  items.forEach((item, index) => {
    const marker = index + 1 === choice ? '>' : ' ';
    console.log(`  ${marker} ${index + 1}. ${item.text}  [${item.objectType}]`);
  });
  console.log(`\nВсего подсказок: ${response.count} · ${response.elapsedMs} мс`);

  let picked = choice;
  if (picked < 1 || picked > items.length) {
    console.log(`\nПодсказки №${choice} нет — беру первую.`);
    picked = 1;
  }

  const item = items[picked - 1]!;

  // Шаг 2. То, что сохраняет бэкенд. Из подсказки нам нужен ТОЛЬКО objectGuid —
  // текст подсказки не хранится, он лишь помог пользователю выбрать объект.
  const card = await getObject(item.objectGuid);

  const codes: Record<string, string> = {};
  // parameters — массив пар typeName/value. Это и есть «нормализованный адрес для
  // БД»: индекс и ОКТМО не надо спрашивать у пользователя, они приезжают сами;
  // код ИФНС и ОКТМО нужны для налоговой отчётности и госформ.
  for (const parameter of card.parameters ?? []) {
    codes[parameter.typeName] = parameter.value;
  }

  return {
    objectGuid: card.objectGuid,
    objectId: card.objectId,
    fullAddress: card.fullAddress,
    objectType: card.objectType,
    regionCode: card.regionCode,
    hierarchy: (card.hierarchy ?? []).map((level) => level.displayName),
    codes,
  };
}

async function main(): Promise<void> {
  if (API_KEY === SANDBOX_KEY) {
    console.log('Демо-ключ: ответы сгенерированы (моки), не реальные данные.\n');
  }

  const prefix = process.argv[2] ?? 'москва тверская';
  const choice = process.argv[3] ? Number(process.argv[3]) : 1;

  const address = await suggestAddress(prefix, choice);

  if (address === null) {
    console.log('\nПодсказок не найдено: адреса нет в ГАР.');
    console.log('Вероятно, опечатка — или новостройка, ещё не внесённая в реестр.');
    return;
  }

  console.log('\n── Карточка для сохранения в БД ──────────────────────────────');
  console.log(`  objectGuid (код ФИАС): ${address.objectGuid}   ← первичный ключ адреса`);
  console.log(`  objectId:              ${address.objectId}`);
  console.log(`  Полный адрес:          ${address.fullAddress}`);
  console.log(`  Тип объекта:           ${address.objectType}`);
  console.log(`  Код региона:           ${address.regionCode}`);

  console.log('\n  Иерархия (регион → город → улица → дом):');
  address.hierarchy.forEach((name, index) => console.log(`    ${index + 1}. ${name}`));

  const codes = Object.entries(address.codes);
  if (codes.length > 0) {
    console.log('\n  Коды (хранить денормализованно рядом с GUID):');
    for (const [name, value] of codes) {
      console.log(`    ${name.padEnd(18)} ${value}`);
    }
  }

  console.log('\nАдрес нормализован. В БД уходит objectGuid, а не строка пользователя:');
  console.log('«ул. Лесная», «улица Лесная» и «Лесная ул.» — это один и тот же objectGuid.');
}

// Запуск только когда файл выполняется напрямую, а не импортируется.
if (process.argv[1]?.includes('index')) {
  main().catch((error: unknown) => {
    console.error('Ошибка:', error instanceof Error ? error.message : error);
    process.exit(1);
  });
}
