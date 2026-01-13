import { Switch } from '@/components/ui';
import { ClientIcon } from '@/components/icons/client-icons';
import type { ClientType } from '@/lib/transport';
import type { ClientConfig } from '../types';

interface ClientsConfigSectionProps {
  clients: ClientConfig[];
  onUpdateClient: (clientId: ClientType, updates: Partial<ClientConfig>) => void;
}

export function ClientsConfigSection({ clients, onUpdateClient }: ClientsConfigSectionProps) {
  return (
    <div>
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        {clients.map((client) => (
          <div
            key={client.id}
            className={`rounded-xl border transition-all duration-200 flex flex-col ${
              client.enabled 
                ? 'bg-surface-primary border-border shadow-sm' 
                : 'bg-surface-secondary/30 border-transparent opacity-80 hover:opacity-100 hover:bg-surface-secondary/50'
            }`}
          >
            <div className="flex items-center justify-between p-4 border-b border-transparent">
              <div className="flex items-center gap-3">
                <ClientIcon type={client.id} size={32} />
                <span className={`text-base font-semibold ${client.enabled ? 'text-text-primary' : 'text-text-secondary'}`}>
                  {client.name}
                </span>
              </div>
              <div onClick={(e) => e.stopPropagation()}>
                <Switch checked={client.enabled} onCheckedChange={(checked) => onUpdateClient(client.id, { enabled: checked })} />
              </div>
            </div>
            
            {/* Expandable/Visible Content */}
            <div className={`px-4 pb-4 transition-all duration-200 ${client.enabled ? 'opacity-100' : 'opacity-50 grayscale pointer-events-none'}`}>
               <div className="bg-surface-secondary/50 rounded-lg p-3 border border-border/50">
                  <label className="text-xs font-medium text-text-secondary block mb-1.5 uppercase tracking-wide">
                     Endpoint Override
                  </label>
                  <input
                    type="text"
                    value={client.urlOverride}
                    onChange={(e) => onUpdateClient(client.id, { urlOverride: e.target.value })}
                    placeholder="Default"
                    disabled={!client.enabled}
                    className="form-input text-sm w-full bg-surface-primary h-9"
                  />
               </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
