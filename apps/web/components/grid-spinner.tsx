"use client";

import { useLingui } from "@lingui/react/macro";

type GridSpinnerProps = {
  className?: string;
};

export function GridSpinner({ className }: GridSpinnerProps) {
  const { t } = useLingui();
  return (
    <span className={`spinner-grid ${className ?? ""}`} role="status" aria-label={t`Loading`}>
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
    </span>
  );
}
