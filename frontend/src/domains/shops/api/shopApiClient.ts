import { buildApiClient } from "../../../api/client/buildApiClient";
import { getToken } from "../../auth/storage";

export const shopApiClient = buildApiClient(getToken);
