import { NavLink, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import type { LucideIcon } from 'lucide-react'
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarMenu,
  SidebarMenuItem,
} from '@/components/ui/sidebar'

interface NavItem {
  to: string
  icon: LucideIcon
  labelKey: string
}

interface NavMainProps {
  items: NavItem[]
  children?: React.ReactNode
}

export function NavMain({ items, children }: NavMainProps) {
  const location = useLocation()
  const { t } = useTranslation()

  return (
    <SidebarGroup>
      <SidebarGroupContent>
        <SidebarMenu>
          {items.map(item => {
            const Icon = item.icon
            const isActive =
              item.to === '/'
                ? location.pathname === '/'
                : location.pathname.startsWith(item.to)

            return (
              <SidebarMenuItem key={item.to}>
                <NavLink
                  to={item.to}
                  data-size="lg"
                  className={({ isActive: linkIsActive }) =>
                    `ring-sidebar-ring hover:bg-sidebar-accent hover:text-sidebar-accent-foreground active:bg-sidebar-accent active:text-sidebar-accent-foreground data-active:bg-sidebar-accent data-active:text-sidebar-accent-foreground gap-2 rounded-md p-2 text-left text-sm transition-[width,height,padding] group-has-data-[sidebar=menu-action]/menu-item:pr-8 group-data-[collapsible=icon]:size-8! group-data-[collapsible=icon]:p-2! focus-visible:ring-2 data-active:font-medium peer/menu-button flex w-full items-center overflow-hidden outline-hidden group/menu-button disabled:pointer-events-none disabled:opacity-50 aria-disabled:pointer-events-none aria-disabled:opacity-50 [&>span:last-child]:truncate [&_svg]:size-4 [&_svg]:shrink-0 h-12 text-sm group-data-[collapsible=icon]:p-0! ${isActive || linkIsActive ? 'bg-sidebar-accent text-sidebar-accent-foreground font-medium' : ''}`
                  }
                >
                  <Icon />
                  <span className="group-data-[collapsible=icon]:hidden">{t(item.labelKey)}</span>
                </NavLink>
              </SidebarMenuItem>
            )
          })}
          {children}
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  )
}
