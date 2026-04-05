import { expect, test } from "playwright/test";

import {
  prioritizeActiveRequests,
} from "../../../web/src/lib/request-order";
import {
  REQUEST_LIST_OVERSCAN,
  calculateVirtualRange,
} from "../../../web/src/pages/requests/virtual-range";

test("virtual range only keeps the visible rows plus overscan in the DOM", () => {
  const range = calculateVirtualRange(100, 1_200, 600, 40);

  expect(range).toEqual({
    startIndex: 22,
    endIndex: 53,
    topSpacerHeight: 880,
    bottomSpacerHeight: 1_880,
  });
});

test("virtual range falls back to rendering all rows when viewport metrics are unavailable", () => {
  const range = calculateVirtualRange(12, 0, 0, 40, REQUEST_LIST_OVERSCAN);

  expect(range).toEqual({
    startIndex: 0,
    endIndex: 12,
    topSpacerHeight: 0,
    bottomSpacerHeight: 0,
  });
});

test("request ordering keeps PENDING and IN_PROGRESS rows ahead of terminal states", () => {
  const ordered = prioritizeActiveRequests([
    { id: 10, status: "COMPLETED" },
    { id: 11, status: "PENDING" },
    { id: 12, status: "FAILED" },
    { id: 13, status: "IN_PROGRESS" },
    { id: 14, status: "CANCELLED" },
  ]);

  expect(ordered.map((item) => item.id)).toEqual([13, 11, 14, 12, 10]);
});
