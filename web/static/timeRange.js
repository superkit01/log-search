function unixSeconds(d) {
  return Math.floor(d.getTime() / 1000);
}

function startOfTodayTs(now) {
  const d = new Date(now || Date.now());
  d.setHours(0, 0, 0, 0);
  return unixSeconds(d);
}

function endOfTodayTs(now) {
  const d = new Date(now || Date.now());
  d.setHours(23, 59, 0, 0);
  return unixSeconds(d);
}

function roundedHoursAgoStartTs(hours, now) {
  const d = new Date(now || Date.now());
  d.setHours(d.getHours() - hours, 0, 0, 0);
  return unixSeconds(d);
}

function quickRangeTimestamps(range, now) {
  const end = endOfTodayTs(now);
  if (range === "today") return { start: startOfTodayTs(now), end };
  if (range === "3h") return { start: roundedHoursAgoStartTs(3, now), end };
  return { start: roundedHoursAgoStartTs(1, now), end };
}

function defaultStartTs(now) {
  return quickRangeTimestamps("1h", now).start;
}

function defaultEndTs(now) {
  return quickRangeTimestamps("1h", now).end;
}

if (typeof module !== "undefined" && module.exports) {
  module.exports = { quickRangeTimestamps, defaultStartTs, defaultEndTs };
}
