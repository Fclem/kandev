"use client";
import { useTranslation } from "react-i18next";
import { IconPlus, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import type { KeyValue } from "@/lib/settings/types";
import { generateUUID } from "@/lib/utils";

type KeyValueInputProps = {
  items: KeyValue[];
  onChange: (items: KeyValue[]) => void;
  keyPlaceholder?: string;
  valuePlaceholder?: string;
  addButtonLabel?: string;
  masked?: boolean;
};

export function KeyValueInput({
  items,
  onChange,
  keyPlaceholder,
  valuePlaceholder,
  addButtonLabel,
  masked = false,
}: KeyValueInputProps) {
  const { t } = useTranslation();
  const keyPlaceholderLabel = keyPlaceholder ?? t("settings:key");
  const valuePlaceholderLabel = valuePlaceholder ?? t("settings:value2");
  const addLabel = addButtonLabel ?? t("settings:addItem");

  const handleAdd = () => {
    onChange([...items, { id: generateUUID(), key: "", value: "" }]);
  };

  const handleRemove = (id: string) => {
    onChange(items.filter((item) => item.id !== id));
  };

  const handleChange = (id: string, field: "key" | "value", value: string) => {
    onChange(items.map((item) => (item.id === id ? { ...item, [field]: value } : item)));
  };

  return (
    <div className="space-y-2">
      {items.map((item) => (
        <div key={item.id} className="flex gap-2">
          <Input
            placeholder={keyPlaceholderLabel}
            value={item.key}
            onChange={(e) => handleChange(item.id, "key", e.target.value)}
            className="flex-1"
          />
          <Input
            type={masked ? "password" : "text"}
            placeholder={valuePlaceholderLabel}
            value={item.value}
            onChange={(e) => handleChange(item.id, "value", e.target.value)}
            className="flex-1"
          />
          <Button variant="ghost" size="icon" onClick={() => handleRemove(item.id)}>
            <IconX className="h-4 w-4" />
          </Button>
        </div>
      ))}
      <Button variant="outline" size="sm" onClick={handleAdd} className="w-full">
        <IconPlus className="h-4 w-4 mr-2" />
        {addLabel}
      </Button>
    </div>
  );
}
