"use client";
import { useTranslation } from "react-i18next";
import { TaskSearchInput } from "./task-search-input";

type MobileSearchBarProps = {
  searchQuery: string;
  onSearchChange: (query: string) => void;
};

export function MobileSearchBar({ searchQuery, onSearchChange }: MobileSearchBarProps) {
  const { t } = useTranslation();
  return (
    <div className="border-b border-border px-4 py-2" data-testid="mobile-search-bar">
      <TaskSearchInput
        value={searchQuery}
        onChange={onSearchChange}
        placeholder={t("kanban:searchTasks2")}
        className="w-full"
        autoFocus
      />
    </div>
  );
}
