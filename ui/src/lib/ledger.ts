const DAY_MS = 86_400_000;
const DAY_KEY = /^(\d{4})-(\d{2})-(\d{2})$/;

export interface LedgerPoint {
  date: string;
  usd: number;
  gramsCO2?: number;
}

export interface LedgerDay {
  date: string;
  usd: number;
  gramsCO2: number;
}

interface LedgerOptions {
  windowDays?: number;
  today?: Date;
}

function utcDayKey(date: Date): string {
  const year = String(date.getUTCFullYear()).padStart(4, "0");
  const month = String(date.getUTCMonth() + 1).padStart(2, "0");
  const day = String(date.getUTCDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function parseUtcDay(key: string): number | null {
  const match = DAY_KEY.exec(key);
  if (!match) return null;
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  const ms = Date.UTC(year, month - 1, day);
  if (utcDayKey(new Date(ms)) !== key) return null;
  return ms;
}

// The API dates each qualifying proposal by its creation day (UTC) and emits one
// point per proposal, so several points can share a date. Buckets must be summed
// per day and zero-filled; plotting raw rows connects unrelated proposals into a
// curve that implies an intra-day trend the ledger does not contain.
export function buildDailyLedger(points: LedgerPoint[], options: LedgerOptions = {}): LedgerDay[] {
  const totals = new Map<string, { usd: number; gramsCO2: number }>();

  for (const point of points) {
    const key = point.date.slice(0, 10);
    if (parseUtcDay(key) === null) continue;
    const entry = totals.get(key) ?? { usd: 0, gramsCO2: 0 };
    const usd = Number(point.usd);
    const grams = Number(point.gramsCO2 ?? 0);
    entry.usd += Number.isFinite(usd) ? usd : 0;
    entry.gramsCO2 += Number.isFinite(grams) ? grams : 0;
    totals.set(key, entry);
  }

  const windowDays = Math.floor(options.windowDays ?? 0);
  let start: number;
  let end: number;

  if (windowDays > 0) {
    const today = options.today ?? new Date();
    end = Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), today.getUTCDate());
    start = end - (windowDays - 1) * DAY_MS;
  } else if (totals.size > 0) {
    const keys = [...totals.keys()].sort();
    start = parseUtcDay(keys[0]) as number;
    end = parseUtcDay(keys[keys.length - 1]) as number;
  } else {
    return [];
  }

  const days: LedgerDay[] = [];
  for (let ms = start; ms <= end; ms += DAY_MS) {
    const key = utcDayKey(new Date(ms));
    const entry = totals.get(key);
    days.push({ date: key, usd: entry?.usd ?? 0, gramsCO2: entry?.gramsCO2 ?? 0 });
  }
  return days;
}

export function formatLedgerDay(key: string, includeYear = false): string {
  const ms = parseUtcDay(key);
  if (ms === null) return key;
  const options: Intl.DateTimeFormatOptions = {
    month: "short",
    day: "numeric",
    timeZone: "UTC",
  };
  if (includeYear) options.year = "2-digit";
  return new Date(ms).toLocaleDateString(undefined, options);
}
