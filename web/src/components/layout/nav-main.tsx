import { NavLink, useLocation } from 'react-router-dom'
import type { LucideIcon } from 'lucide-react'
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarMenu,
  SidebarMenuItem,
  SidebarMenuButton,
} from '@/components/ui/sidebar'

interface NavItem {
  to: string
  icon: LucideIcon
  label: string
}

interface NavMainProps {
  items: NavItem[]
  children?: React.ReactNode
}

export function NavMain({ items, children }: NavMainProps) {
  const location = useLocation()

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
                <SidebarMenuButton
                  render={props => <NavLink to={item.to} {...props} />}
                  isActive={isActive}
                  size="lg"
                  className="min-w-8 duration-200 ease-linear"
                >
                  <Icon />
                  <span>{item.label}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            )
          })}
          {children}
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  )
}
