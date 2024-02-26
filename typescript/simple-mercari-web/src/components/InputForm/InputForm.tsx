import { ComponentPropsWithoutRef, FC } from "react";

type InputFormProps = ComponentPropsWithoutRef<"input"> & {
  label: string;
};

export const InputForm: FC<InputFormProps> = ({
  id,
  type = "text",
  name,
  placeholder,
  label,
  onChange,
}) => {
  return (
    <label className="InputForm">
      {label}
      <input
        id={id}
        type={type}
        name={name}
        placeholder={placeholder}
        onChange={onChange}
      />
    </label>
  );
};
