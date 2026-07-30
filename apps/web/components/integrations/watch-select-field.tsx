"use client";

import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useTranslation } from "react-i18next";
import { resolveOptionLabel, type OptionLabel } from "@/lib/i18n/option-label";

/**
 * One option in a watch dialog's select. Carries a catalog key for options we
 * author and a verbatim label for runtime values (repositories, labels, agent
 * profiles) — see {@link OptionLabel}. Always render through
 * {@link resolveOptionLabel}: reading `item.label` directly renders blank for a
 * key-carrying option, and the optional fields let that past the type checker.
 */
export type WatchSelectItem = OptionLabel & { id: string; icon?: React.ReactNode };

export type WatchSelectFieldProps = {
  /** Already-translated field label. */
  label: string;
  /** Already-translated helper text. */
  description?: string;
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
  items: WatchSelectItem[];
  disabled?: boolean;
};

/**
 * The labelled select used across the integration watch dialogs. Extracted from
 * `issue-watch-dialog` / `review-watch-dialog`, which each carried a copy.
 */
export function WatchSelectField({
  label,
  description,
  value,
  onChange,
  placeholder,
  items,
  disabled,
}: WatchSelectFieldProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <Label>{label}</Label>
      {description && <p className="text-xs text-muted-foreground">{description}</p>}
      <Select value={value || undefined} onValueChange={onChange} disabled={disabled}>
        <SelectTrigger className="cursor-pointer">
          <SelectValue placeholder={placeholder} />
        </SelectTrigger>
        <SelectContent>
          {items.map((item) => (
            <SelectItem key={item.id} value={item.id}>
              {item.icon ? (
                <span className="flex items-center gap-1.5">
                  <span>{resolveOptionLabel(t, item)}</span>
                  {item.icon}
                </span>
              ) : (
                resolveOptionLabel(t, item)
              )}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
