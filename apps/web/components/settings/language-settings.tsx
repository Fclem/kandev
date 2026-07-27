"use client";

import { useState } from "react";
import { Trans, useLingui } from "@lingui/react/macro";
import { IconLanguage } from "@tabler/icons-react";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { SettingsSection } from "@/components/settings/settings-section";
import { SettingsCard } from "@/components/settings/settings-card";
import {
  activateLocale,
  i18n,
  LOCALE_LABELS,
  normalizeLocale,
  selectableLocales,
  type SupportedLocale,
} from "@/lib/i18n";

/**
 * Language switcher. Applies immediately (no deferred save) — `activateLocale`
 * re-renders the app and persists the choice to the `kandev_locale` cookie — so
 * this surface owns no unsaved state and does not register a save contributor.
 * The pseudo QA locale is hidden in production builds.
 */
export function LanguageSettings() {
  const { t } = useLingui();
  const [locale, setLocale] = useState<SupportedLocale>(() => normalizeLocale(i18n.locale));

  const options = selectableLocales(import.meta.env.PROD);

  const handleChange = async (value: string) => {
    const activated = await activateLocale(value);
    setLocale(activated);
  };

  return (
    <SettingsSection
      icon={<IconLanguage className="h-5 w-5" />}
      title={t`Language`}
      description={t`Choose the language used across the Kandev interface`}
    >
      <SettingsCard>
        <div className="flex flex-col gap-2">
          <Label htmlFor="language-select">
            <Trans>Display language</Trans>
          </Label>
          <Select value={locale} onValueChange={handleChange}>
            <SelectTrigger id="language-select" className="w-64" aria-label={t`Display language`}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {options.map((candidate) => (
                <SelectItem key={candidate} value={candidate}>
                  {LOCALE_LABELS[candidate]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-sm text-muted-foreground">
            <Trans>
              Changes the language of the Kandev interface. Applies immediately and is remembered on
              this device. Task titles, chat, and other content you create are not translated.
            </Trans>
          </p>
        </div>
      </SettingsCard>
    </SettingsSection>
  );
}
