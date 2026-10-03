"""
Клиент API ГАР/ФИАС Atlorium — поиск и нормализация российских адресов.

Запуск (работает сразу, без регистрации — на демо-ключе):
    pip install -r requirements.txt
    python main.py "москва тверская"

Второй аргумент — номер выбранной подсказки (по умолчанию 1):
    python main.py "москва тверская" 2

Боевой ключ: получить на https://atlorium.com и положить в переменную окружения
ATLORIUM_API_KEY. Код при этом не меняется.
"""

import os
import sys
from dataclasses import dataclass, field

import requests

# Публичный демо-ключ. С ним API отвечает правдоподобными МОКАМИ (не реальными
# данными ГАР) — чтобы можно было встроить и протестировать интеграцию до оплаты.
# Ответы детерминированы: один и тот же запрос всегда даёт один и тот же результат,
# поэтому на них можно писать стабильные тесты.
SANDBOX_KEY = "ak_sandbox_demo_mockdata_v1"

API_KEY = os.environ.get("ATLORIUM_API_KEY", SANDBOX_KEY)
BASE_URL = os.environ.get("ATLORIUM_BASE_URL", "https://atlorium.com")

TIMEOUT = 30


class AtloriumError(RuntimeError):
    """Ошибка API. Код HTTP разложен в человекочитаемую причину."""

    REASONS = {
        400: "Неверный запрос (пустая строка поиска или некорректный GUID/objectId)",
        401: "API-ключ отсутствует, просрочен или недействителен",
        402: "Недостаточно кредитов на балансе — пополните на https://atlorium.com",
        404: "Адресный объект не найден в ГАР/ФИАС",
        429: "Превышен лимит запросов — повторите позже",
        500: "Внутренняя ошибка при обращении к адресной базе (за сбой на своей стороне мы не списываем деньги)",
        503: "Сервис временно недоступен (плановые работы) — повторите позже",
    }

    def __init__(self, status: int, body: str):
        reason = self.REASONS.get(status, "Неизвестная ошибка")
        super().__init__(f"HTTP {status}: {reason}. Ответ сервера: {body[:200]}")
        self.status = status


def _get(path: str, params: dict | None = None) -> dict:
    response = requests.get(
        f"{BASE_URL}{path}",
        params=params,
        headers={
            "Authorization": f"Bearer {API_KEY}",
            "Accept": "application/json",
        },
        timeout=TIMEOUT,
    )
    if not response.ok:
        raise AtloriumError(response.status_code, response.text)
    return response.json()


# ── Обёртки над эндпоинтами ──────────────────────────────────────────────────


def suggest(query: str, limit: int = 7) -> dict:
    """Автодополнение: быстрые подсказки по префиксу. Элементы лежат в `items`."""
    return _get("/api/Gar/suggest", {"query": query, "limit": limit})


def search(query: str, limit: int = 10) -> dict:
    """Полный поиск. Элементы лежат в `results`, есть ОКТМО/ОКАТО/индекс/код региона."""
    return _get("/api/Gar/search", {"query": query, "limit": limit})


def get_object(guid: str) -> dict:
    """Карточка объекта по GUID ФИАС: иерархия + параметры (индекс, ОКТМО, ОКАТО, ИФНС, КЛАДР)."""
    return _get(f"/api/Gar/object/{guid}")


def get_object_by_id(object_id: int) -> dict:
    """Карточка объекта по числовому objectId."""
    return _get(f"/api/Gar/object/id/{object_id}")


def get_hierarchy(object_id: int) -> dict:
    """Путь объекта от региона до дома."""
    return _get(f"/api/Gar/hierarchy/{object_id}")


def get_children(parent_object_id: int, limit: int = 50) -> dict:
    """Дочерние объекты: улицы региона, дома улицы и т.д."""
    return _get(f"/api/Gar/children/{parent_object_id}", {"limit": limit})


def get_regions() -> dict:
    """Список регионов РФ."""
    return _get("/api/Gar/regions")


def get_stats() -> dict:
    """Статистика адресной базы."""
    return _get("/api/Gar/stats")


def get_region_stats(region_code: str) -> dict:
    """Статистика по конкретному региону."""
    return _get(f"/api/Gar/region/{region_code}/stats")


# ── Применение данных: автодополнение адреса в форме ──────────────────────────
# Ответ API сам по себе — просто JSON. Ценность появляется, когда из него собирают
# то, что реально уезжает в базу. Ниже — ровно то, что делает связка «фронтенд +
# бэкенд» в форме заказа или регистрации:
#
#   Шаг 1 (фронтенд): по префиксу, который набрал пользователь, дёргаем /suggest и
#           показываем выпадающий список подсказок.
#   Шаг 2 (бэкенд): у ВЫБРАННОЙ подсказки берём objectGuid и дёргаем /object/{guid} —
#           получаем канонический адрес и коды.
#
# ГЛАВНОЕ: в БД нельзя хранить строку, которую набрал пользователь. Хранить надо
# objectGuid (код ФИАС) — стабильный идентификатор, — а рядом денормализованно
# fullAddress, индекс, ОКТМО, ОКАТО, ИФНС, КЛАДР. Тогда «ул. Лесная», «улица Лесная»
# и «Лесная ул.» — это один objectGuid, а не три разных адреса в базе.


@dataclass
class NormalizedAddress:
    """Карточка, готовая к сохранению в БД."""

    object_guid: str  # ← первичный ключ адреса. Именно он хранится в базе
    object_id: int
    full_address: str
    object_type: str
    region_code: str
    hierarchy: list[str] = field(default_factory=list)
    codes: dict[str, str] = field(default_factory=dict)  # индекс, ОКТМО, ОКАТО, ИФНС, КЛАДР


def suggest_address(prefix: str, choice: int = 1) -> NormalizedAddress | None:
    """Автодополнение адреса, доведённое до конца: подсказки → канонический объект.

    Возвращает None, если подсказок нет: такого адреса нет в ГАР — вероятно
    опечатка или новостройка, ещё не внесённая в реестр.
    """
    # Шаг 1. То, что видит пользователь: выпадающий список под полем ввода.
    response = suggest(prefix, limit=7)
    items = response.get("items") or []

    print(f'Ввод пользователя: «{prefix}»\n')
    print("Подсказки (выпадающий список под полем ввода):")
    if not items:
        print("  — пусто")
        return None

    for index, item in enumerate(items, start=1):
        marker = ">" if index == choice else " "
        print(f'  {marker} {index}. {item["text"]}  [{item.get("objectType")}]')
    print(f'\nВсего подсказок: {response.get("count")} · {response.get("elapsedMs")} мс')

    if choice < 1 or choice > len(items):
        print(f"\nПодсказки №{choice} нет — беру первую.")
        choice = 1

    picked = items[choice - 1]

    # Шаг 2. То, что сохраняет бэкенд. Из подсказки нам нужен ТОЛЬКО objectGuid —
    # текст подсказки не хранится, он лишь помог пользователю выбрать объект.
    card = get_object(picked["objectGuid"])

    return NormalizedAddress(
        object_guid=card["objectGuid"],
        object_id=card["objectId"],
        full_address=card["fullAddress"],
        object_type=card.get("objectType", ""),
        region_code=card.get("regionCode", ""),
        hierarchy=[level["displayName"] for level in card.get("hierarchy") or []],
        # parameters — массив пар typeName/value. Это и есть «нормализованный адрес
        # для БД»: индекс и ОКТМО не надо спрашивать у пользователя, они приезжают
        # сами; код ИФНС и ОКТМО нужны для налоговой отчётности и госформ.
        codes={p["typeName"]: p["value"] for p in card.get("parameters") or []},
    )


def main() -> int:
    if API_KEY == SANDBOX_KEY:
        print("Демо-ключ: ответы сгенерированы (моки), не реальные данные.\n")

    prefix = sys.argv[1] if len(sys.argv) > 1 else "москва тверская"
    choice = int(sys.argv[2]) if len(sys.argv) > 2 else 1

    try:
        address = suggest_address(prefix, choice)
    except AtloriumError as error:
        print(f"Ошибка: {error}", file=sys.stderr)
        return 1

    if address is None:
        print("\nПодсказок не найдено: адреса нет в ГАР.")
        print("Вероятно, опечатка — или новостройка, ещё не внесённая в реестр.")
        return 0

    print("\n── Карточка для сохранения в БД ──────────────────────────────")
    print(f"  objectGuid (код ФИАС): {address.object_guid}   ← первичный ключ адреса")
    print(f"  objectId:              {address.object_id}")
    print(f"  Полный адрес:          {address.full_address}")
    print(f"  Тип объекта:           {address.object_type}")
    print(f"  Код региона:           {address.region_code}")

    print("\n  Иерархия (регион → город → улица → дом):")
    for level, name in enumerate(address.hierarchy, start=1):
        print(f"    {level}. {name}")

    if address.codes:
        print("\n  Коды (хранить денормализованно рядом с GUID):")
        for name, value in address.codes.items():
            print(f"    {name:<18} {value}")

    print("\nАдрес нормализован. В БД уходит objectGuid, а не строка пользователя:")
    print("«ул. Лесная», «улица Лесная» и «Лесная ул.» — это один и тот же objectGuid.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
