import { describe, expect, it } from "vitest";
import config from "../playwright.config";
import {
  DESKTOP_COARSE_POINTER_VIEWPORT,
  MOBILE_FINE_POINTER_VIEWPORT,
  projectContextOptions,
} from "../fixtures/project-contexts";
import { NORMAL_PROJECT_NAMES } from "./native-catalog";

/**
 * Project-cohort regressions for the normal (non-container) Playwright
 * projects.
 *
 * These read the *real* config instead of restating it: the shard planner
 * discovers its catalog by asking Playwright for exactly the projects in
 * `NORMAL_PROJECT_NAMES`, so a spec file that matched two normal projects would
 * be planned twice, and a project missing from that list would be planned not
 * at all. The context assertions cover the other half: `testPage` opens its own
 * context, so the fixture — not the project's `use` block — decides the
 * viewport and pointer a spec actually runs with.
 */

type ProjectEntry = {
  name: string;
  testMatch?: unknown;
  testIgnore?: unknown;
  use?: {
    viewport?: { width: number; height: number } | null;
    hasTouch?: boolean;
    isMobile?: boolean;
  };
};

const projects = config.projects as unknown as ProjectEntry[];
const SPEC_FILE_PATTERN = /\.(?:spec|test)\.[cm]?[jt]sx?$/;

function patterns(matcher: unknown): RegExp[] {
  if (matcher === undefined) return [];
  const values = Array.isArray(matcher) ? matcher : [matcher];
  return values.map((value) => {
    if (value instanceof RegExp) return value;
    throw new Error(`this regression only understands RegExp matchers, got ${String(value)}`);
  });
}

function matches(project: ProjectEntry, file: string): boolean {
  const matchers = patterns(project.testMatch);
  const matched =
    matchers.length > 0
      ? matchers.some((pattern) => pattern.test(file))
      : SPEC_FILE_PATTERN.test(file);
  if (!matched) return false;
  return !patterns(project.testIgnore).some((pattern) => pattern.test(file));
}

function projectsForFile(file: string): string[] {
  return projects
    .filter((project) => matches(project, file))
    .map((project) => project.name)
    .sort();
}

function projectByName(name: string): ProjectEntry {
  const project = projects.find((candidate) => candidate.name === name);
  if (!project) throw new Error(`playwright.config.ts declares no ${name} project`);
  return project;
}

describe("playwright project cohort", () => {
  it("declares every project the shard planner plans for", () => {
    const declared = projects.map((project) => project.name);
    for (const name of NORMAL_PROJECT_NAMES) {
      expect(declared, `${name} must exist in playwright.config.ts`).toContain(name);
    }
  });

  it("routes each repository-star spec file to exactly one project", () => {
    expect(
      projectsForFile("tests/settings/plugin-repository-stars.spec.ts"),
      "the desktop fine-pointer star spec belongs to chromium alone",
    ).toEqual(["chromium"]);
    expect(
      projectsForFile("tests/settings/desktop-coarse-pointer-plugin-repository-stars.spec.ts"),
      "the coarse-pointer star spec must not also run as a fine-pointer chromium file",
    ).toEqual(["desktop-coarse-pointer"]);
    expect(projectsForFile("tests/settings/mobile-plugin-repository-stars.spec.ts")).toEqual([
      "mobile-chrome",
    ]);
    expect(
      projectsForFile("tests/settings/mobile-fine-pointer-plugin-repository-stars.spec.ts"),
      "the 767px fine-pointer star spec must not also be catalogued under mobile-chrome",
    ).toEqual(["mobile-fine-pointer"]);
    expect(projectsForFile("tests/auth/plugin-repository-stars.spec.ts")).toEqual(["auth"]);
  });

  it("keeps ordinary mobile and desktop specs on their existing projects", () => {
    expect(projectsForFile("tests/chat/example.spec.ts")).toEqual(["chromium"]);
    expect(projectsForFile("tests/chat/mobile-example.spec.ts")).toEqual(["mobile-chrome"]);
    expect(projectsForFile("tests/canvas/mobile-canvas-marketplace.spec.ts")).toEqual([
      "mobile-chrome",
    ]);
  });

  it("emulates the viewport and pointer each added project is named for", () => {
    // Pixel 5 emulation is preserved byte for byte: mobile, touch, phone width.
    const phone = projectContextOptions("mobile-chrome");
    expect(phone.isMobile).toBe(true);
    expect(phone.hasTouch).toBe(true);
    expect(phone.viewport?.width).toBeLessThan(768);
    expect(phone.deviceScaleFactor).toBeGreaterThan(1);

    // 767px is the largest mobile-classified width; the pointer stays fine, so
    // the 44px touch target there is viewport-driven, not touch-driven.
    expect(projectContextOptions("mobile-fine-pointer")).toEqual({
      viewport: { ...MOBILE_FINE_POINTER_VIEWPORT },
      deviceScaleFactor: 1,
      hasTouch: false,
      isMobile: false,
    });

    // Desktop width with touch: Chromium reports a coarse primary pointer.
    expect(projectContextOptions("desktop-coarse-pointer")).toEqual({
      viewport: { ...DESKTOP_COARSE_POINTER_VIEWPORT },
      deviceScaleFactor: 1,
      hasTouch: true,
      isMobile: false,
    });

    // Everything else keeps Playwright's own desktop fine-pointer defaults.
    expect(projectContextOptions("chromium")).toEqual({});
    expect(projectContextOptions("routing")).toEqual({});
  });

  it("applies the same emulation in the project definitions", () => {
    const coarse = projectByName("desktop-coarse-pointer").use;
    expect(coarse?.hasTouch).toBe(true);
    expect(coarse?.isMobile).toBe(false);
    expect(coarse?.viewport).toEqual({ ...DESKTOP_COARSE_POINTER_VIEWPORT });

    const fine = projectByName("mobile-fine-pointer").use;
    expect(fine?.hasTouch).toBe(false);
    expect(fine?.viewport).toEqual({ ...MOBILE_FINE_POINTER_VIEWPORT });

    const desktop = projectByName("chromium").use;
    expect(desktop?.viewport?.width).toBeGreaterThanOrEqual(1024);
    expect(desktop?.hasTouch).toBeFalsy();
  });
});
