import {
  IconInbox,
  IconAt,
  IconGitPullRequest,
  IconPencil,
  IconGitMerge,
  IconPlus,
  IconCircleCheck,
} from "@tabler/icons-react";
import type { Icon } from "@tabler/icons-react";
import type { OptionLabel } from "@/lib/i18n/option-label";

export type PresetGroup = "inbox" | "created";

export type PresetOption = OptionLabel & {
  value: string;
  filter: string;
  group: PresetGroup;
  icon: Icon;
};

export const PR_PRESETS: PresetOption[] = [
  {
    value: "review_requested",
    labelKey: "github:reviewRequested",
    filter: "review-requested:@me is:open",
    group: "inbox",
    icon: IconInbox,
  },
  {
    value: "mentioned",
    labelKey: "github:mentions",
    filter: "mentions:@me is:open",
    group: "inbox",
    icon: IconAt,
  },
  {
    value: "open",
    labelKey: "github:open",
    filter: "author:@me is:open",
    group: "created",
    icon: IconGitPullRequest,
  },
  {
    value: "drafts",
    labelKey: "github:drafts",
    filter: "author:@me is:open draft:true",
    group: "created",
    icon: IconPencil,
  },
  {
    value: "merged",
    labelKey: "github:recentlyMerged",
    filter: "author:@me is:merged",
    group: "created",
    icon: IconGitMerge,
  },
];

export const ISSUE_PRESETS: PresetOption[] = [
  {
    value: "assigned",
    labelKey: "github:assigned",
    filter: "assignee:@me is:open",
    group: "inbox",
    icon: IconInbox,
  },
  {
    value: "mentioned",
    labelKey: "github:mentions",
    filter: "mentions:@me is:open",
    group: "inbox",
    icon: IconAt,
  },
  {
    value: "created",
    labelKey: "github:open",
    filter: "author:@me is:open",
    group: "created",
    icon: IconPlus,
  },
  {
    value: "closed",
    labelKey: "github:recentlyClosed",
    filter: "author:@me is:closed",
    group: "created",
    icon: IconCircleCheck,
  },
];
