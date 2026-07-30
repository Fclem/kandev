"use client";

import { Trans } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";

import type { FormState } from "./slack-settings-form-state";

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
  return (
    <div className="space-y-4">
      <div className="space-y-1.5">
        <Label htmlFor="slack-token">
          <Trans
            i18nKey="common:sessionTokenXoxc"
            values={{
              value1: hasSavedToken && (
                <span className="text-xs text-muted-foreground ml-2">
                  (saved — leave blank to keep)
                </span>
              ),
            }}
          >
            Session token (xoxc-…)
            {hasSavedToken && (
              <span className="text-xs text-muted-foreground ml-2">
                (saved — leave blank to keep)
              </span>
            )}
          </Trans>
        </Label>
        <Input
          id="slack-token"
          type="password"
          placeholder={hasSavedToken ? "••••••••" : "xoxc-..."}
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
                <span className="text-xs text-muted-foreground ml-2">
                  (saved — leave blank to keep)
                </span>
              ),
            }}
          >
            d cookie value
            {hasSavedCookie && (
              <span className="text-xs text-muted-foreground ml-2">
                (saved — leave blank to keep)
              </span>
            )}
          </Trans>
        </Label>
        <Input
          id="slack-cookie"
          type="password"
          placeholder={hasSavedCookie ? "••••••••" : "xoxd-..."}
          value={form.cookie}
          data-settings-dirty={form.cookie !== baseline.cookie}
          onChange={(e) => update("cookie", e.target.value)}
          disabled={loading}
        />
        <p className="text-xs text-muted-foreground">
          Open Slack in your browser, copy the value of the `d` cookie and the `xoxc-` token from
          local storage. Both are required.
        </p>
      </div>
    </div>
  );
}
