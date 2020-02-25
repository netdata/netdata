/** @deprecated since v6.3.0 - use constants property exposed by the relevant module instead. */
declare module "constants" {
    import { constants as osConstants, SignalConstants } from 'os';
    import { constants as cryptoConstants } from 'crypto';
    import { constants as fsConstants } from 'fs';
    const exp: typeof osConstants.errno & typeof osConstants.priority & SignalConstants & typeof cryptoConstants & typeof fsConstants;
    export = exp;
}
