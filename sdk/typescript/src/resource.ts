export interface ResourceContent {
  uri: string;
  text?: string;
  blob?: Uint8Array;
  mimeType?: string;
}

export interface ResourceDef {
  uri: string;
  name: string;
  description: string;
  handler: (uri: string) => ResourceContent | ResourceContent[] | string;
  mimeType?: string;
  size?: number;
}

export interface ResourceTemplateDef {
  uriTemplate: string;
  name: string;
  description: string;
  handler: (uri: string) => ResourceContent | ResourceContent[] | string;
  mimeType?: string;
}

const resourceRegistry: ResourceDef[] = [];
const templateRegistry: ResourceTemplateDef[] = [];

export interface ResourceOptions {
  uri: string;
  description: string;
  name?: string;
  mimeType?: string;
  handler: (uri: string) => ResourceContent | ResourceContent[] | string;
}

export function resource(options: ResourceOptions): ResourceDef {
  const def: ResourceDef = {
    uri: options.uri,
    name: options.name ?? options.uri,
    description: options.description,
    handler: options.handler,
    mimeType: options.mimeType,
  };
  resourceRegistry.push(def);
  return def;
}

export interface ResourceTemplateOptions {
  uriTemplate: string;
  description: string;
  name?: string;
  mimeType?: string;
  handler: (uri: string) => ResourceContent | ResourceContent[] | string;
}

export function resourceTemplate(options: ResourceTemplateOptions): ResourceTemplateDef {
  const def: ResourceTemplateDef = {
    uriTemplate: options.uriTemplate,
    name: options.name ?? options.uriTemplate,
    description: options.description,
    handler: options.handler,
    mimeType: options.mimeType,
  };
  templateRegistry.push(def);
  return def;
}

export function getRegisteredResources(): ResourceDef[] {
  return [...resourceRegistry];
}

export function getRegisteredResourceTemplates(): ResourceTemplateDef[] {
  return [...templateRegistry];
}

export function clearResourceRegistry(): void {
  resourceRegistry.length = 0;
}

export function clearTemplateRegistry(): void {
  templateRegistry.length = 0;
}
