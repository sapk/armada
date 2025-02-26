import React from "react"
import { Button, IconButton } from "@material-ui/core"
import RefreshIcon from '@material-ui/icons/Refresh'
import CancelIcon from "@material-ui/icons/Cancel"

import ColumnSelect from "./ColumnSelect";
import { ColumnSpec } from "../../containers/JobsContainer";

import './JobTableHeader.css'

type JobTableHeaderProps = {
  defaultColumns: ColumnSpec<string | boolean | string[]>[]
  annotationColumns: ColumnSpec<string>[]
  canCancel: boolean
  onRefresh: () => void
  onCancelJobsClick: () => void
  onDisableColumn: (id: string, isDisabled: boolean) => void
  onDeleteColumn: (columnId: string) => void
  onAddColumn: () => void
  onChangeAnnotationColumnKey: (columnId: string, newKey: string) => void
}

export default function JobTableHeader(props: JobTableHeaderProps) {
  return (
    <div className="job-table-header">
      <div className="left">
        <h2 className="title">Jobs</h2>
      </div>
      <div className="right">
        <div className="select-columns">
          <ColumnSelect
            defaultColumns={props.defaultColumns}
            annotationColumns={props.annotationColumns}
            inputLabel={"Annotation key"}
            addColumnText={"Add column for annotation"}
            onDisableColumn={props.onDisableColumn}
            onDeleteColumn={props.onDeleteColumn}
            onAddColumn={props.onAddColumn}
            onChangeAnnotationColumnKey={props.onChangeAnnotationColumnKey}/>
        </div>
        <div className="cancel-jobs">
          <Button
            disabled={!props.canCancel}
            variant="contained"
            color="secondary"
            startIcon={<CancelIcon/>}
            onClick={props.onCancelJobsClick}>
            Cancel
          </Button>
        </div>
        <div className="refresh">
          <IconButton onClick={props.onRefresh} color="primary">
            <RefreshIcon/>
          </IconButton>
        </div>
      </div>
    </div>
  )
}
