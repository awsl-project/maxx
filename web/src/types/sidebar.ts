import type { LucideIcon } from 'lucide-react';

/**
 * Standard menu item with navigation link
 */
export interface StandardMenuItem {
  type: 'standard';
  key: string;
  to: string;
  icon: LucideIcon;
  labelKey: string;
  activeMatch?: 'exact' | 'startsWith'; // Default: 'startsWith'
  adminOnly?: boolean; // Only show for admin role
}

/**
 * Custom menu item with React component
 */
export interface CustomMenuItem {
  type: 'custom';
  key: string;
  component: React.ComponentType;
}

/**
 * Dynamic section that generates multiple items
 */
export interface DynamicSectionMenuItem {
  type: 'dynamic-section';
  key: string;
  generator: () => React.ReactNode;
}

/**
 * Union type for all menu item types
 */
export type MenuItem = StandardMenuItem | CustomMenuItem | DynamicSectionMenuItem;

/**
 * Sidebar section grouping menu items
 */
export interface SidebarSection {
  key: string;
  titleKey?: string; // Translation key for section title
  items: MenuItem[];
}

/**
 * Root sidebar configuration
 */
export interface SidebarConfig {
  sections: SidebarSection[];
}
