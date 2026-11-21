import { useMemo, useRef, useState, useSyncExternalStore } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "../../../components/ui/Button";
import { shrinkPhoto, type PhotoLimits } from "../../../lib/photoResize";
import styles from "./photoField.module.css";

function formatMB(bytes: number): string {
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

// 選んだ File の一時プレビュー URL を外部リソースとして購読する。StrictMode の再購読でも破棄済みの URL を掴まず、
// photo の差し替え・アンマウント時には確実に revoke する。
function createObjectUrlStore(photo: File | null) {
  let currentUrl: string | null = null;
  return {
    subscribe(notify: () => void) {
      if (photo) {
        currentUrl = URL.createObjectURL(photo);
        notify();
      }
      return () => {
        if (currentUrl) {
          URL.revokeObjectURL(currentUrl);
          currentUrl = null;
        }
      };
    },
    getSnapshot() {
      return photo ? currentUrl : null;
    },
  };
}

function useObjectUrl(photo: File | null): string | null {
  const store = useMemo(() => createObjectUrlStore(photo), [photo]);
  return useSyncExternalStore(store.subscribe, store.getSnapshot, () => null);
}

interface Props {
  // 選んだ(小さくしたあとの)写真。まだ選んでいなければ null。
  photo: File | null;
  onChange: (photo: File | null) => void;
  // 送る前に、上限を超える写真だけを、この端末で小さくするための値(GET /meta の photo)。未取得の間は、
  // 選んだ写真をそのまま使う(小さくせず、backend の判断に任せる)。
  limits: PhotoLimits | undefined;
  // 小さくしている間だけ true にする(送信の可否は、呼び出し側がこれで判断する)。
  onShrinkingChange?: (shrinking: boolean) => void;
  // 編集で、まだ新しい写真を選んでいないときに見せる、いまの写真(なければ null)。
  existingPhotoUrl?: string | null;
  // いまの写真の下に出す説明(例: 投稿日)。existingPhotoUrl があるときだけ使う。
  existingPhotoCaption?: string;
  // 外す操作を出すか(投稿: 出す / 編集: 出さない。デザインの review-edit.html は「変える」だけ)。
  allowRemove?: boolean;
}

// 写真の追加欄(design/redesign/review-new.html・review-photo.html・review-edit.html)。空・選んだ直後(この
// 端末で小さくしている間)・付いた、の 3 つの見た目を、1 つの部品で扱う。小さくする処理そのものは
// lib/photoResize の shrinkPhoto(ここで一度だけ行う。送信の直前にはもう一度呼ばない。二重にデコードしないため)。
export function PhotoField({ photo, onChange, limits, onShrinkingChange, existingPhotoUrl, existingPhotoCaption, allowRemove = false }: Props) {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLInputElement>(null);
  const [shrinking, setShrinking] = useState(false);
  const [originalSize, setOriginalSize] = useState<number | null>(null);
  // 選び直しを見分ける通し番号。速く終わった、あとの選択の結果を、遅れて終わった前の選択の結果が
  // 上書きしないようにする(2 つの写真を続けて選んだときの、非同期処理の追い越しへの対策)。
  const selection = useRef(0);
  const objectUrl = useObjectUrl(photo);

  const handleSelect = async (file: File) => {
    const mySelection = ++selection.current;
    setOriginalSize(file.size);
    if (!limits) {
      onChange(file);
      return;
    }
    setShrinking(true);
    onShrinkingChange?.(true);
    try {
      const shrunk = await shrinkPhoto(file, limits);
      if (mySelection === selection.current) onChange(shrunk);
    } finally {
      if (mySelection === selection.current) {
        setShrinking(false);
        onShrinkingChange?.(false);
      }
    }
  };

  const remove = () => {
    onChange(null);
    setOriginalSize(null);
    if (inputRef.current) inputRef.current.value = "";
  };

  const previewUrl = photo ? objectUrl : existingPhotoUrl;
  const showsExisting = !photo && !!existingPhotoUrl;

  return (
    <div className={styles.field}>
      <span className={styles.label}>
        {t("reviews.photo.label")} <span className={styles.optional}>{t("reviews.photo.optional")}</span>
      </span>
      <input
        ref={inputRef}
        type="file"
        accept="image/jpeg,image/png,image/webp"
        className={styles.hiddenInput}
        onChange={(e) => {
          const file = e.target.files?.[0];
          // 同じファイルを続けて選び直したときも change イベントが発火するよう、読み取り後に input の値を空に戻す。
          e.currentTarget.value = "";
          if (file) void handleSelect(file);
        }}
      />
      {shrinking ? (
        <div className={styles.drop} aria-live="polite">
          <b className={styles.titleSm}>{t("reviews.photo.shrinking")}</b>
        </div>
      ) : previewUrl ? (
        <div>
          <div className={styles.preview}>
            <img src={previewUrl} alt={showsExisting ? t("reviews.photo.currentAlt") : t("reviews.photo.alt")} className={styles.previewImg} />
          </div>
          <div className={styles.file}>
            <span className={styles.fname}>{showsExisting ? t("reviews.photo.currentFile") : photo?.name}</span>
            <span className={styles.muted}>
              {showsExisting
                ? existingPhotoCaption
                : originalSize !== null && photo && originalSize !== photo.size
                  ? t("reviews.photo.shrunkFrom", { from: formatMB(originalSize), to: formatMB(photo.size) })
                  : null}
            </span>
          </div>
          <div className={styles.buttons}>
            <Button type="button" variant="secondary" onClick={() => inputRef.current?.click()}>
              {t("reviews.photo.change")}
            </Button>
            {allowRemove && photo && (
              <Button type="button" variant="secondary" onClick={remove}>
                {t("reviews.photo.remove")}
              </Button>
            )}
          </div>
        </div>
      ) : (
        <div className={styles.drop}>
          <b className={styles.titleSm}>{t("reviews.photo.add")}</b>
          <p className={styles.hint}>{t("reviews.photo.hint")}</p>
          <Button type="button" variant="secondary" onClick={() => inputRef.current?.click()}>
            {t("reviews.photo.choose")}
          </Button>
        </div>
      )}
    </div>
  );
}
