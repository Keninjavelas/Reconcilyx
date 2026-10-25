import test from "node:test";
import assert from "node:assert/strict";
import { buildDailyLedger } from "../src/lib/ledger.ts";

test("same-day entries collapse into a single summed bucket", () => {
  const days = buildDailyLedger([
    { date: "2026-10-24", usd: 18.25, gramsCO2: 4200 },
    { date: "2026-10-24", usd: 0 },
    { date: "2026-10-24", usd: 1.5, gramsCO2: 300 },
  ]);

  assert.equal(days.length, 1);
  assert.deepEqual(days[0], { date: "2026-10-24", usd: 19.75, gramsCO2: 4500 });
});

test("gaps between recorded days are zero-filled in calendar order", () => {
  const days = buildDailyLedger([
    { date: "2026-10-23", usd: 3, gramsCO2: 100 },
    { date: "2026-10-20", usd: 2 },
  ]);

  assert.deepEqual(
    days.map((day) => day.date),
    ["2026-10-20", "2026-10-21", "2026-10-22", "2026-10-23"],
  );
  assert.deepEqual(
    days.map((day) => day.usd),
    [2, 0, 0, 3],
  );
});

test("windowDays ends today (UTC), spans exactly that many days, and excludes outside records", () => {
  const today = new Date("2026-10-25T23:59:59Z");
  const days = buildDailyLedger(
    [
      { date: "2026-10-24", usd: 18.25, gramsCO2: 4200 },
      { date: "2026-09-01", usd: 99 },
      { date: "2026-10-26", usd: 5 },
    ],
    { windowDays: 30, today },
  );

  assert.equal(days.length, 30);
  assert.equal(days[0].date, "2026-09-26");
  assert.equal(days[days.length - 1].date, "2026-10-25");
  assert.equal(days.find((day) => day.date === "2026-10-24")?.usd, 18.25);
  assert.equal(
    days.reduce((sum, day) => sum + day.usd, 0),
    18.25,
  );
});

test("window buckets exist even when no record falls inside the window", () => {
  const days = buildDailyLedger(
    [{ date: "2026-01-01", usd: 10 }],
    { windowDays: 7, today: new Date("2026-10-25T12:00:00Z") },
  );

  assert.equal(days.length, 7);
  assert.ok(days.every((day) => day.usd === 0 && day.gramsCO2 === 0));
});

test("impossible and malformed dates are ignored", () => {
  const days = buildDailyLedger([
    { date: "2026-02-31", usd: 2 },
    { date: "not-a-date", usd: 1 },
    { date: "", usd: 4 },
  ]);

  assert.deepEqual(days, []);
});

test("daily totals reconcile with the recorded entries", () => {
  const input = [
    { date: "2026-10-24", usd: 12.4, gramsCO2: 3000 },
    { date: "2026-10-25", usd: 6.1, gramsCO2: 1200 },
    { date: "2026-10-25", usd: 4.9, gramsCO2: 800 },
  ];
  const days = buildDailyLedger(input);

  assert.equal(
    days.reduce((sum, day) => sum + day.usd, 0),
    input.reduce((sum, point) => sum + point.usd, 0),
  );
  assert.equal(
    days.reduce((sum, day) => sum + day.gramsCO2, 0),
    input.reduce((sum, point) => sum + (point.gramsCO2 ?? 0), 0),
  );
});
