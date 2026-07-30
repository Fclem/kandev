"use client";

import { Trans, useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";

import type { FormState } from "./slack-settings-form-state";

/** Hint styling for the "saved — leave blank to keep" note beside each field. */
const SAVED_HINT_CLASS = "text-xs text-muted-foreground ml-2";

/** Key for the shared "saved — leave blank to keep" hint. */
const SAVED_HINT_KEY = "common:savedLeaveBlankToKeep";

// Bot-token / signing-secret inputs for the Slack settings form. Extracted to
// keep slack-settings.tsx under the 600-line limit.
type SecretFieldsProps = {
  form: FormState;
  baseline: FormState;
  loading: boolean;
  hasSavedToken: boolean;
  hasSavedCookie: boolean;
  update: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
};

export function SecretFields({
  form,
  baseline,
  loading,
  hasSavedToken,
  hasSavedCookie,
  update,
}: SecretFieldsProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      <div className="space-y-1.5">
        <Label htmlFor="slack-token">
          <Trans
            i18nKey="common:sessionTokenXoxc"
            values={{
              value1: hasSavedToken && (
                <span className={SAVED_HINT_CLASS}>{t(SAVED_HINT_KEY)}</span>
              ),
            }}
          >
            {t("common:sessionTokenXoxc2")}
            {hasSavedToken && <span className={SAVED_HINT_CLASS}>{t(SAVED_HINT_KEY)}</span>}
          </Trans>
        </Label>
        <Input
          id="slack-token"
          type="password"
          placeholder={hasSavedToken ? "••••••••" : t("common:xoxc")}
          value={form.token}
          data-settings-dirty={form.token !== baseline.token}
          onChange={(e) => update("token", e.target.value)}
          disabled={loading}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="slack-cookie">
          <Trans
            i18nKey="common:dCookieValue"
            values={{
              value1: hasSavedCookie && (
                <span className={SAVED_HINT_CLASS}>{t(SAVED_HINT_KEY)}</span>
              ),
            }}
          >
            d cookie value
            {hasSavedCookie && <span className={SAVED_HINT_CLASS}>{t(SAVED_HINT_KEY)}</span>}
          </Trans>
        </Label>
        <Input
          id="slack-cookie"
          type="password"
          placeholder={hasSavedCookie ? "••••••••" : t("common:xoxd")}
          value={form.cookie}
          data-settings-dirty={form.cookie !== baseline.cookie}
          onChange={(e) => update("cookie", e.target.value)}
          disabled={loading}
        />
        <p className="text-xs text-muted-foreground">{t("common:openSlackInYourBrowserCopy")}</p>
      </div>
    </div>
  );
}
