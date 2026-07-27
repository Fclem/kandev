import * as React from "react";

import { cn } from "./lib/utils";
import { useUIStrings } from "./lib/ui-strings";
import { Button } from "./button";
import { IconChevronLeft, IconChevronRight, IconDots } from "@tabler/icons-react";

function Pagination({ className, ...props }: React.ComponentProps<"nav">) {
  return (
    <nav
      role="navigation"
      aria-label="pagination"
      data-slot="pagination"
      className={cn("mx-auto flex w-full justify-center", className)}
      {...props}
    />
  );
}

function PaginationContent({ className, ...props }: React.ComponentProps<"ul">) {
  return (
    <ul
      data-slot="pagination-content"
      className={cn("gap-0.5 flex items-center", className)}
      {...props}
    />
  );
}

function PaginationItem({ ...props }: React.ComponentProps<"li">) {
  return <li data-slot="pagination-item" {...props} />;
}

type PaginationLinkProps = {
  isActive?: boolean;
} & Pick<React.ComponentProps<typeof Button>, "size"> &
  React.ComponentProps<"a">;

function PaginationLink({ className, isActive, size = "icon", ...props }: PaginationLinkProps) {
  return (
    <Button asChild variant={isActive ? "outline" : "ghost"} size={size} className={cn(className)}>
      <a
        aria-current={isActive ? "page" : undefined}
        data-slot="pagination-link"
        data-active={isActive}
        {...props}
      />
    </Button>
  );
}

function PaginationPrevious({ className, ...props }: React.ComponentProps<typeof PaginationLink>) {
  const uiStrings = useUIStrings();
  return (
    <PaginationLink
      aria-label={uiStrings.goToPreviousPage}
      size="default"
      className={cn("pl-2!", className)}
      {...props}
    >
      <IconChevronLeft data-icon="inline-start" />
      <span className="hidden sm:block">{uiStrings.previous}</span>
    </PaginationLink>
  );
}

function PaginationNext({ className, ...props }: React.ComponentProps<typeof PaginationLink>) {
  const uiStrings = useUIStrings();
  return (
    <PaginationLink
      aria-label={uiStrings.goToNextPage}
      size="default"
      className={cn("pr-2!", className)}
      {...props}
    >
      <span className="hidden sm:block">{uiStrings.next}</span>
      <IconChevronRight data-icon="inline-end" />
    </PaginationLink>
  );
}

function PaginationEllipsis({ className, ...props }: React.ComponentProps<"span">) {
  const uiStrings = useUIStrings();
  return (
    <span
      aria-hidden
      data-slot="pagination-ellipsis"
      className={cn(
        "size-7 items-center justify-center [&_svg:not([class*='size-'])]:size-3.5 flex items-center justify-center",
        className,
      )}
      {...props}
    >
      <IconDots />
      <span className="sr-only">{uiStrings.morePages}</span>
    </span>
  );
}

export {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
};
