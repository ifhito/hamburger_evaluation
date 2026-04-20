import { buildApiClient } from "../../../api/client/buildApiClient";
import { getToken } from "../../auth/storage";

export const userApiClient = buildApiClient(getToken);
