function mirrorShape(
  target: object,
  source: schemas.$ZodObject,
  keys: PropertyKey[],
  wrap?: ((value: any, key: PropertyKey) => any) | null
): void {
  const raw = sourceShape(source);
  for (const key of keys) {
    const desc = Object.getOwnPropertyDescriptor(raw, key)!;
    if (!desc.enumerable) continue;
    if (desc.get) {
      deferProp(target, key, () => {
        const value = (source._zod.def.shape as any)[key];
        return wrap ? wrap(value, key) : value;
      });
    } else putProp(target, key, wrap ? wrap(desc.value, key) : desc.value);
  }
}

// same, for a plain shape a caller passed rather than a schema's

function mirrorProps(target: object, source: Record<PropertyKey, any>): void {
  for (const key of Reflect.ownKeys(source)) {
    const desc = Object.getOwnPropertyDescriptor(source, key)!;
    if (!desc.enumerable) continue;
    if (desc.get) deferProp(target, key, () => source[key as any]);
    else putProp(target, key, desc.value);
  }
}
