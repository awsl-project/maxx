import type { ProxyRequest } from '@/lib/transport';

export interface RequestModelChain {
  requestModel: string;
  mappedModel: string;
  title: string;
}

export function getRequestModelChain(
  request: Pick<ProxyRequest, 'requestModel' | 'mappedModel'>,
): RequestModelChain {
  const requestModel = request.requestModel || '';
  const mappedModel =
    request.mappedModel && request.mappedModel !== requestModel ? request.mappedModel : '';
  const title = mappedModel
    ? `Request model: ${requestModel || '-'}\nMapped model: ${mappedModel}`
    : requestModel;

  return {
    requestModel,
    mappedModel,
    title,
  };
}
