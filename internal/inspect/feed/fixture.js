/* Deterministic fixture for the card 275 timeline prototype.
 *
 * Produces window.ANAM_FIXTURE = {w, live, entries} in the shape of a GET /v1/timeline answer
 * (entries oldest first). Seeded PRNG, fixed end date 2026-11-07, zone +11:00 (Australia/Sydney),
 * so two loads give the same page. Replace this whole file with
 *   window.ANAM_FIXTURE = <a saved real answer>;
 * to see real data (see README.md). window.ANAM_FIXTURE_TAIL is optional: three later entries
 * the page offers as "3 new since you opened this" in live mode.
 */
(function () {
  'use strict';

  // mulberry32: small seeded PRNG, returns floats in [0, 1).
  function mulberry32(a) {
    return function () {
      a |= 0; a = (a + 0x6D2B79F5) | 0;
      var t = Math.imul(a ^ (a >>> 15), 1 | a);
      t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
      return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
  }
  var rnd = mulberry32(275275);
  function ri(a, b) { return a + Math.floor(rnd() * (b - a + 1)); }
  function pick(a) { return a[Math.floor(rnd() * a.length)]; }
  function pad(n, w) { var s = String(n); while (s.length < (w || 2)) s = '0' + s; return s; }
  function hex(n) { var s = ''; for (var i = 0; i < n; i++) s += Math.floor(rnd() * 16).toString(16); return s; }
  function r1(x) { return Math.round(x * 10) / 10; }

  var START = Date.UTC(2025, 8, 7);   // day 0, a Sunday
  var END_DAY = 426;                  // 2026-11-07
  var OFFSET = '+11:00';
  var CUTOFF = '2026-11-07T09:35';    // "now" on the last day, nothing later than this

  // Local wall-clock ISO string for (day index, minutes since local midnight), with the offset.
  function ts(day, min) {
    return new Date(START + day * 864e5 + min * 6e4).toISOString().slice(0, 19) + OFFSET;
  }

  var entries = [];
  var seq = 0;
  var CLASS = { health: 'Sensitive', money: 'Private' };

  function add(type, day, min, source, payload, o) {
    o = o || {};
    seq++;
    var e = {
      facet_id: o.id || ('f_' + pad(seq.toString(16), 4) + hex(6)),
      type: type,
      fact_time: ts(day, min),
      source: source,
      source_ref: o.ref || (source + '-' + pad(seq, 5)),
      status: o.status || 'live',
      classification: CLASS[type.split('.')[0]] || 'Normal',
      content_hash: 'sha256:' + hex(8) + hex(8) + hex(8) + hex(8) + hex(8) + hex(8) + hex(8) + hex(8),
      payload: payload
    };
    entries.push(e);
    return e;
  }

  var NOTES = [
    'Slept badly, the neighbour was loud again.', 'Good session, felt strong on the squats.',
    'Need to ring the dentist about the cracked tooth.', 'Coffee with Priya, she is moving to Perth in March.',
    'Reminder to self: renew the car registration before the 20th.', 'Long walk along the harbour, clear head.',
    'The new desk chair is better than expected.', 'Tried the sourdough again, still too dense.',
    'Book idea: a history of weights and measures.', 'Knee a bit sore after the run, ice tonight.',
    'Dinner at Mum\'s, she looks well.', 'Quiet day. Caught up on email and reading.',
    'Tax receipts folder is a mess, sort it this week.', 'Read the first three chapters of the Hickey essays.'
  ];
  var BOAT_NOTES = [
    'Hull survey done, a few soft spots near the keel.', 'Ordered antifoul, the yard says two coats minimum.',
    'Spoke to the rigger, standing rigging is due in the spring.', 'Engine starts first time now, new injectors made the difference.',
    'The boat: marina fees went up again next quarter.', 'Sanded the transom all afternoon.',
    'Chart plotter works, sea trial planned for the weekend.', 'Bilge pump replaced. One less thing to worry about.'
  ];
  var MERCH = [['Woolworths', 25, 140], ['Coles', 20, 120], ['Bunnings', 15, 190], ['Uber', 12, 45], ['Chemist Warehouse', 10, 60],
    ['Dan Murphy\'s', 25, 90], ['Bills Cafe', 8, 32], ['Kmart', 15, 80], ['Officeworks', 12, 70], ['Sydney Fish Market', 20, 65]];
  var BOAT_MERCH = [['Whitworths Marine', 30, 480], ['Bunnings', 18, 160], ['Hull Paint Supplies', 90, 340], ['Marina Berth Fees', 380, 380], ['Boat Rigging Co', 120, 650]];
  var PLACES = ['Home', 'Office, Barangaroo', 'Cafe on Crown St', 'Bondi Beach', 'Mum\'s place', 'Priya\'s flat', 'Library, Surry Hills', 'Marina, Rushcutters Bay', 'Fitness centre'];
  var VENUES = ['Anytime Fitness Newtown', 'Fitness First Pyrmont', 'Bondi Icebergs'];
  var DOCS = [['Marina berth agreement 2026', 'application/pdf'], ['Hull survey report', 'application/pdf'], ['Car registration renewal', 'application/pdf'],
    ['Tax return 2025 notice of assessment', 'application/pdf'], ['Blood test results', 'application/pdf'], ['Lease renewal', 'application/pdf'],
    ['Whiteboard photo, project plan', 'image/jpeg'], ['Receipt, rigging deposit', 'image/png']];

  var weightKg = 86.4;
  var notesForEdges = [];   // [{note, target}]
  var empty = function (d) { return d >= 200 && d <= 206; };   // the fully empty week
  var QUIET = 420;                                              // the quiet day (sleep and weight only)
  var HR_DAYS = { 423: 1, 350: 1, 150: 1 };                     // the heart-rate repetition days
  var boatDays = {};
  (function () { var d = 272; for (var i = 0; i < 12; i++) { boatDays[d] = i; d += 9 + (i % 3); } })();
  var taxDays = { 300: 1, 331: 1, 359: 1 };

  function addNoteWithEdge(day, min, text, target, edgeText) {
    var n = add('record.note', day, min, 'claude-code', { text: text });
    var ed = add('record.edge', day, min + 1, 'claude-code', { kind: 'about', from: n.facet_id, to: target.facet_id, text: edgeText });
    return ed;
  }

  for (var d = 0; d <= END_DAY; d++) {
    if (empty(d)) continue;
    var dow = d % 7;
    var quiet = d === QUIET;

    // Weight: most mornings; day 410 carries the correction (superseded then live, same facet_id).
    weightKg = weightKg - 0.012 + (rnd() - 0.5) * 0.5;
    if (d === 410) {
      var wid = 'f_weightfix01';
      add('health.weight', d, 7 * 60 + 14, 'withings', { kg: 83.9, source_time: ts(d, 7 * 60 + 14) }, { id: wid, status: 'superseded', ref: 'wt-fix-1' });
      add('health.weight', d, 7 * 60 + 14, 'withings', { kg: 82.9, source_time: ts(d, 7 * 60 + 14) }, { id: wid, status: 'live', ref: 'wt-fix-1' });
    } else if (quiet || rnd() < 0.6) {
      add('health.weight', d, ri(6 * 60 + 20, 8 * 60), 'withings', { kg: r1(weightKg), source_time: ts(d, 7 * 60) });
    }

    // Sleep: starts the evening before wake-up, always crosses midnight; fact_time is the start.
    var sMin = ri(21 * 60 + 30, 23 * 60 + 59), sLen = ri(360, 500);
    var sEnd = ts(d, sMin + sLen);
    add('health.sleep', d, sMin, 'garmin', { period_start: ts(d, sMin), period_end: sEnd, minutes: sLen });

    if (quiet) continue;

    if (rnd() < 0.2) add('health.blood-pressure', d, ri(7 * 60, 8 * 60 + 30), 'withings', { systolic: ri(112, 138), diastolic: ri(68, 88), pulse: ri(54, 78) });
    if (d !== END_DAY && rnd() < 0.7) add('activity.steps', d, 23 * 60 + 50, 'garmin', { count: ri(3200, 14800) });

    // Medication: one drug twice daily for 60 days; a second started on day 100 and stopped on day 250.
    if (d >= 360 && d <= 420) {
      add('health.medication.dose', d, 8 * 60 + ri(0, 12), 'manual', { drug: 'Metoprolol', amount: 25, unit: 'mg' });
      add('health.medication.dose', d, 20 * 60 + ri(0, 12), 'manual', { drug: 'Metoprolol', amount: 25, unit: 'mg' });
    }
    if (d === 100) add('health.medication.started', d, 8 * 60 + 15, 'manual', { drug: 'Sertraline', amount: 50, unit: 'mg' });
    if (d === 250) add('health.medication.stopped', d, 9 * 60, 'manual', { drug: 'Sertraline', reason: 'course complete' });

    // Heart rate: every 5 to 15 minutes across the whole day on three days (hundreds of rows).
    if (HR_DAYS[d]) {
      for (var m = 0; m < 1440; m += ri(5, 15)) {
        var base = 56 + 14 * Math.sin((m - 360) / 1440 * 2 * Math.PI);
        add('health.heart-rate', d, m, 'garmin', { bpm: Math.max(46, Math.round(base + (m > 17 * 60 && m < 18 * 60 ? 40 : 0) + (rnd() - 0.5) * 14)) });
      }
    }

    // Gym: Mon, Wed, Sat, mostly.
    var gym = null;
    if ((dow === 1 || dow === 3 || dow === 6) && rnd() < 0.9) {
      gym = add('activity.gym-visit', d, ri(6 * 60 + 30, 18 * 60 + 30), 'garmin', { venue: pick(VENUES), minutes: ri(40, 85) });
    }

    // Stays: a few a week; one in four is a night away crossing midnight.
    if (rnd() < 0.36) {
      var arr = ri(9 * 60, 19 * 60), dep, overnight = rnd() < 0.25;
      if (overnight) { arr = ri(19 * 60, 22 * 60); dep = arr + ri(8 * 60, 12 * 60); } else { dep = arr + ri(40, 300); }
      add('place.stay', d, arr, 'stays', { place: pick(PLACES), arrived: ts(d, arr), departed: ts(d, dep) });
    }
    // The five-day trip.
    if (d === 380) add('place.trip.departed', d, 6 * 60 + 40, 'owntracks', { destination: 'Melbourne', mode: 'flight' });
    if (d === 385) add('place.trip.returned', d, 18 * 60 + 5, 'owntracks', { destination: 'Melbourne', mode: 'flight' });

    // The project thread: "the boat", a dozen start/end pairs over about four months.
    var bi = boatDays[d];
    var attention = null;
    if (bi !== undefined) {
      var aS = ri(18 * 60, 20 * 60);
      attention = add('mind.attention.started', d, aS, 'claude-code', { topic: 'the boat' });
      add('mind.attention.ended', d, aS + ri(40, 170), 'claude-code', { topic: 'the boat' });
    }
    if (taxDays[d]) {
      var tS = ri(19 * 60, 20 * 60 + 30);
      add('mind.attention.started', d, tS, 'claude-code', { topic: 'taxes' });
      add('mind.attention.ended', d, tS + ri(30, 90), 'claude-code', { topic: 'taxes' });
    }

    // Purchases: a few a week. On the boat days one is tagged to the topic and gets a linked note.
    var boatBuy = null;
    if (bi !== undefined) {
      var bm = BOAT_MERCH[bi % BOAT_MERCH.length];
      boatBuy = add('money.purchase', d, ri(10 * 60, 16 * 60), 'up-bank', { merchant: bm[0], total: r1(bm[1] + rnd() * (bm[2] - bm[1])), currency: 'AUD', topic: 'the boat' });
    }
    if (rnd() < 0.36 || d === 415) {
      var mm = pick(MERCH);
      var pur = add('money.purchase', d, ri(8 * 60, 21 * 60), 'up-bank', { merchant: mm[0], total: r1(mm[1] + rnd() * (mm[2] - mm[1])), currency: 'AUD' },
        d === 415 ? { status: 'retracted' } : {});
      if (d === 415) pur.payload.merchant = 'Duplicate charge, Kmart';
    }

    // Notes: a few a week. Some link to a purchase, gym visit or attention span by an edge.
    if (rnd() < 0.36) {
      var nm = ri(7 * 60, 22 * 60);
      var roll = rnd();
      if (gym && roll < 0.5) addNoteWithEdge(d, nm, pick(NOTES.slice(0, 2).concat(['Knee a bit sore after the run, ice tonight.'])), gym, 'about this gym visit');
      else add('record.note', d, nm, rnd() < 0.5 ? 'claude-code' : 'web', { text: pick(NOTES) });
    }
    if (boatBuy) addNoteWithEdge(d, 16 * 60 + 30 + ri(0, 60), BOAT_NOTES[bi % BOAT_NOTES.length], boatBuy, 'about this purchase');
    if (attention) addNoteWithEdge(d, 21 * 60 + 30, BOAT_NOTES[(bi + 3) % BOAT_NOTES.length], attention, 'written during this span of attention');

    if (rnd() < 0.035) { var dc = pick(DOCS); add('record.document', d, ri(9 * 60, 17 * 60), 'paperless', { title: dc[0], mime: dc[1] }); }

    // The unfamiliar type, absent from the presentation table.
    if (d === 120) add('vehicle.service', d, 10 * 60 + 30, 'manual', { odometer_km: 48210, workshop: 'Inner West Auto', items: ['oil and filter', 'brake pads (front)', 'cabin filter'] });
    if (d === 330) add('vehicle.service', d, 9 * 60 + 5, 'manual', { odometer_km: 56480, workshop: 'Inner West Auto', items: ['oil and filter', 'wiper blades', 'tyre rotation'] });
  }

  // Keep "today" to the morning, and drop edges whose ends were dropped.
  var kept = entries.filter(function (e) { return e.fact_time.slice(0, 16) <= CUTOFF; });
  var ids = {};
  kept.forEach(function (e) { ids[e.facet_id] = 1; });
  kept = kept.filter(function (e) { return e.type !== 'record.edge' || (ids[e.payload.from] && ids[e.payload.to]); });

  kept.sort(function (a, b) {
    return a.fact_time < b.fact_time ? -1 : a.fact_time > b.fact_time ? 1 : a.facet_id < b.facet_id ? -1 : a.facet_id > b.facet_id ? 1 : 0;
  });

  // The tail: entries recorded after the page "opened", offered in live mode.
  entries = [];
  add('record.note', END_DAY, 10 * 60 + 2, 'claude-code', { text: 'Marina called, the berth is confirmed from December.' });
  add('money.purchase', END_DAY, 10 * 60 + 10, 'up-bank', { merchant: 'Blue Bottle Coffee', total: 6.5, currency: 'AUD' });
  add('health.blood-pressure', END_DAY, 10 * 60 + 25, 'withings', { systolic: 121, diastolic: 77, pulse: 62 });
  var tail = entries;

  window.ANAM_FIXTURE = { w: 82031, live: true, entries: kept };
  window.ANAM_FIXTURE_TAIL = tail;
})();
