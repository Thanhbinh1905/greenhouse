export const $ZodTransform: core.$constructor<$ZodTransform> = /*@__PURE__*/ core.$constructor(
  "$ZodTransform",
  (inst, def) => {
    $ZodType.init(inst, def);
    inst._zod.optin = "optional";
    core.globalConfig.memoizer?.guard(inst);

    inst._zod.parse = (payload, ctx) => {
      if (ctx.direction === "backward") {
        throw new core.$ZodEncodeError(inst.constructor.name);
      }

      const _out = def.transform(payload.value, payload);
      if (ctx.async) {
        const output = _out instanceof Promise ? _out : Promise.resolve(_out);
        return output.then((output) => {
          payload.value = output;
          return payload;
        });
      }

      if (_out instanceof Promise) {
        throw new core.$ZodAsyncError();
      }

      payload.value = _out;
      return payload;
    };
  }
);

////////////////////////////////////////////
////////////////////////////////////////////
//////////                        //////////
//////////      $ZodOptional      //////////
//////////                        //////////
////////////////////////////////////////////
////////////////////////////////////////////

function handleCodecTxResult(left: ParsePayload, value: any, nextSchema: SomeType, ctx: ParseContextInternal) {
  // Check if transform added any issues
  if (left.issues.length) {
    left.aborted = true;
    return left;
  }

  return nextSchema._zod.run({ value, issues: left.issues }, ctx);
}

/////////////////////////////////////////////////
/////////////////////////////////////////////////
//////////                             //////////
//////////      $ZodPreprocess         //////////
//////////                             //////////
/////////////////////////////////////////////////
/////////////////////////////////////////////////

