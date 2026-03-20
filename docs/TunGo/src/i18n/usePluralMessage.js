import {usePluralForm} from '@docusaurus/theme-common';

export function usePluralMessage() {
  const {selectMessage} = usePluralForm();

  return (count, pluralizedMessage) => selectMessage(count, pluralizedMessage);
}
