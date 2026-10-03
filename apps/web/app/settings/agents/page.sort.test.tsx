import { describe, expect, it } from "vitest";
import { sortProfileIdsByName } from "@/lib/settings/agent-profile-order";
import type { AgentProfile } from "@/lib/types/http";

const profile = (id: string, name: string) => ({ id, name }) as AgentProfile;

describe("installed agent profile sort action", () => {
  it("uses the active locale's case-insensitive numeric collation and stable ties", () => {
    const profiles = [
      profile("ten", "Profile 10"),
      profile("two", "profile 2"),
      profile("same-a", "Same"),
      profile("same-b", "same"),
    ];
    expect(sortProfileIdsByName(profiles, "en")).toEqual(["two", "ten", "same-a", "same-b"]);
  });
});
