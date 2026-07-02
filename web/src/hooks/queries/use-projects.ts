/**
 * Project React Query Hooks
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { getTransport, type Project, type CreateProjectData } from '@/lib/transport';

// Query Keys
export const projectKeys = {
  all: ['projects'] as const,
  lists: () => [...projectKeys.all, 'list'] as const,
  list: () => [...projectKeys.lists()] as const,
  details: () => [...projectKeys.all, 'detail'] as const,
  detail: (id: number) => [...projectKeys.details(), id] as const,
  slugs: () => [...projectKeys.all, 'slug'] as const,
  slug: (slug: string) => [...projectKeys.slugs(), slug] as const,
};

// 获取所有 Projects
export function useProjects() {
  return useQuery({
    queryKey: projectKeys.list(),
    queryFn: () => getTransport().getProjects(),
  });
}

// 获取单个 Project by ID
export function useProject(id: number) {
  return useQuery({
    queryKey: projectKeys.detail(id),
    queryFn: () => getTransport().getProject(id),
    enabled: id > 0,
  });
}

// 获取单个 Project by Slug
export function useProjectBySlug(slug: string) {
  return useQuery({
    queryKey: projectKeys.slug(slug),
    queryFn: () => getTransport().getProjectBySlug(slug),
    enabled: !!slug,
  });
}

// 创建 Project
export function useCreateProject() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateProjectData) => getTransport().createProject(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectKeys.lists() });
    },
  });
}

// 更新 Project
export function useUpdateProject() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: Partial<Project> }) =>
      getTransport().updateProject(id, data),
    onSuccess: (_, { id }) => {
      queryClient.invalidateQueries({ queryKey: projectKeys.detail(id) });
      queryClient.invalidateQueries({ queryKey: projectKeys.lists() });
    },
  });
}

// 删除 Project
export function useDeleteProject() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: number) => getTransport().deleteProject(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectKeys.lists() });
    },
  });
}

// 归档非活跃 Projects
export function useArchiveInactiveProjects() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (thresholdDays: number) => getTransport().archiveInactiveProjects(thresholdDays),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectKeys.lists() });
    },
  });
}
