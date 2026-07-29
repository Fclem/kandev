"use client";

import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import type { Tier } from "@/lib/state/slices/office/types";
import { useTranslation } from "react-i18next";

const TIER_OPTIONS: Array<{ value: Tier; label: string; hint: string }> = [
  { value: "frontier", label: "Frontier", hint: "Best capability per provider" },
  { value: "balanced", label: "Balanced", hint: "Standard capability" },
  { value: "economy", label: "Economy", hint: "Cheapest viable model" },
];

type Props = {
  value: Tier;
  onChange: (v: Tier) => void;
  disabled?: boolean;
};

export function DefaultTierSelector({ value, onChange, disabled }: Props) {
  const { t } = useTranslation();
  return (
    <div className="rounded-lg border border-border p-4 space-y-3">
      <div>
        <p className="text-sm font-medium">{t("office:defaultTier")}</p>
        <p className="text-xs text-muted-foreground mt-0.5">
          {t("office:agentsInheritThisTierUnlessThey")}
        </p>
      </div>
      <ToggleGroup
        type="single"
        value={value}
        onValueChange={(v) => v && onChange(v as Tier)}
        disabled={disabled}
        className="justify-start"
      >
        {TIER_OPTIONS.map((opt) => (
          <ToggleGroupItem
            key={opt.value}
            value={opt.value}
            className="cursor-pointer flex flex-col items-center px-4 py-2 h-auto"
            title={opt.hint}
          >
            <span className="text-sm">{opt.label}</span>
            <span className="text-[10px] text-muted-foreground">{opt.hint}</span>
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
    </div>
  );
}
